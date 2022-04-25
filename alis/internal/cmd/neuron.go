package cmd

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	pbProducts "go.protobuf.alis.alis.exchange/alis/os/resources/products/v1"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var (
	pushProtocolBuffers        bool
	genprotoPython             bool
	genprotoGo                 bool
	genprotoCpp                bool
	setNeuronDeploymentEnvFlag bool
	setUpdateNeuronEnvFlag     bool
	setUpdateNeuronStateFlag   bool
	setDeployNeuronStateFlag   bool
	publishApiFlag             bool
)

type Parameters struct {
	Organisation string
	Product      string
	Contract     string
	Neuron       string
	VersionMajor string
}

// neuronCmd represents the neuron command
var neuronCmd = &cobra.Command{
	Use:   "neuron",
	Short: pterm.Blue("Manages neurons within your product"),
	Long:  pterm.Green(`Use this command to update, deploy, create, delete neurons within your product resource.`),
	Run: func(cmd *cobra.Command, args []string) {
		pterm.Error.Println("a valid command is missing\nplease run 'alis neuron -h' for details.")
	},
}

// createNeuronCmd represents the create command
var createNeuronCmd = &cobra.Command{
	Use:   "create",
	Short: pterm.Blue("Creates a new neuron"),
	Long: pterm.Green(
		`This method creates a new neuron in the specified product.

It creates a new Neuron resource, and adds boiler plate proto and product repository
files to get your started with.  Once you have a first version of your service, commit
the changes to the master branch and run the command "alis neuron build ..." `),
	Example: pterm.LightYellow("alis neuron create {orgID}.{productID}.{neuronID}"),
	Args:    validateNeuronArg,
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]
		neuronID = strings.Split(args[0], ".")[2]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			// TODO: handle not found by listing available organisations.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the product resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			// TODO: handle not found by listing available products.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetProduct:\n%s\n", product)

		// Check if neuron exists
		_, err = alisProductsClient.GetNeuron(cmd.Context(), &pbProducts.GetNeuronRequest{
			Name: "organisations/" + organisationID + "/products/" + productID + "/neurons/" + neuronID})
		if err == nil {
			pterm.Error.Println("neuron already exits.")
			return
		}

		envs, err := askUserNeuronEnvs(nil)
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// Retrieve the neuron resource
		op, err := alisProductsClient.CreateNeuron(cmd.Context(),
			&pbProducts.CreateNeuronRequest{
				Parent: product.GetName(),
				Neuron: &pbProducts.Neuron{
					Type: pbProducts.Neuron_RESOURCE,
					Envs: envs,
				},
				NeuronId: neuronID,
			})
		if err != nil {
			// TODO: handle not found by listing available products.
			pterm.Error.Println(err)
			return
		}

		// wait for the long-running operation to complete.
		err = wait(cmd.Context(), op, "Creating "+neuronID, "Created "+neuronID, 300, true)
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// retrieve a copy of the neuron
		neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
			&pbProducts.GetNeuronRequest{Name: "organisations/" + organisationID + "/products/" + productID + "/neurons/" + neuronID})
		if err != nil {
			// TODO: handle not found by listing available products.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetNeuron:\n%s\n", neuron)

		// push boiler plate code to local environment
		// Parse the template files.
		files, err := TemplateFs.ReadDir("templates/go")
		if err != nil {
			return
		}
		pterm.Info.Printf("Created the following files:\n")
		for i, f := range files {

			fileTemplate, err := TemplateFs.ReadFile("templates/go/" + f.Name())
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			t, err := template.New(fmt.Sprintf("%v", i)).Parse(string(fileTemplate))
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			// A temporary workaround for the .mod file templates.
			filename := strings.Replace(f.Name(), ".tmpl", "", -1)

			var destDir string
			switch {
			case strings.HasSuffix(filename, ".proto") || strings.HasSuffix(filename, ".tf"):
				// save in proto repository
				destDir = fmt.Sprintf("%s/alis.exchange/%s/%s/%s/%s/%s", homeDir, organisationID, "proto", organisationID, productID, strings.ReplaceAll(neuronID, "-", "/"))
			default:
				destDir = fmt.Sprintf("%s/alis.exchange/%s/products/%s/%s", homeDir, organisationID, productID, strings.ReplaceAll(neuronID, "-", "/"))
			}

			err = os.MkdirAll(destDir, os.FileMode(0777))
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			file, err := os.Create(fmt.Sprintf("%s/%s", destDir, filename))
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			// set the parameters, the project defaults to {organisation}-{product}-dev
			p := Parameters{
				Organisation: organisationID,
				Product:      productID,
				Contract:     strings.Split(neuronID, "-")[0],
				Neuron:       strings.Split(neuronID, "-")[1],
				VersionMajor: strings.Split(neuronID, "-")[2],
			}
			err = t.Execute(file, p)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			pterm.Printf("%s%s/%s\n", pterm.Cyan(" ● "), destDir, filename)
		}
		ptermTip.Printf("The above files have been added to your proto and product repositories, but have "+
			"not yet been committed.\nMake the necessary changes to the files, commit them to the master before running "+
			"the `alis neuron build %s.%s.%s` command\n", organisationID, productID, neuronID)
	},
}

// getNeuronCmd represents the get command
var getNeuronCmd = &cobra.Command{
	Use:     "get",
	Short:   pterm.Blue("Retrieve details on a specified neuron."),
	Example: pterm.LightYellow("alis neuron list {orgID}.{productID}.{neuronID}"),
	Args:    validateNeuronArg,
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]
		neuronID = strings.Split(args[0], ".")[2]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the product resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetProduct:\n%s\n", product)

		// Retrieve the neuron resource
		neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
			&pbProducts.GetNeuronRequest{Name: "organisations/" + organisationID +
				"/products/" + productID + "/neurons/" + neuronID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetNeuron:\n%s\n", neuron)

		// Retrieve Product deployments
		productsDeploymentsRes, err := alisProductsClient.ListProductDeployments(cmd.Context(), &pbProducts.ListProductDeploymentsRequest{
			Parent: product.GetName(),
		})
		productDeployments := productsDeploymentsRes.GetProductDeployments()
		pterm.Debug.Printf("ListProductDeployments:\n%v found\n", len(productsDeploymentsRes.GetProductDeployments()))

		// Retrieve the latest neuronVersion
		listNeuronVersionsRes, err := alisProductsClient.ListNeuronVersions(cmd.Context(), &pbProducts.ListNeuronVersionsRequest{
			Parent: neuron.GetName(),
		})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		neuronVersions := listNeuronVersionsRes.GetNeuronVersions()
		var neuronVersion *pbProducts.NeuronVersion
		if len(neuronVersions) > 0 {
			neuronVersion = listNeuronVersionsRes.GetNeuronVersions()[0]
		}

		// Generate table with Neuron details.
		pterm.DefaultSection.Print("NEURON BUILD:")
		// Color the state
		state := neuronVersion.GetState().String()
		switch neuronVersion.GetState() {
		case pbProducts.NeuronVersion_FAILED:
			state = pterm.Red(state)
		}
		header := []string{"Resource ID", "Latest Build Version", "Update Time", "State", "Resource Name"}
		row := []string{
			neuronID, neuronVersion.GetVersion(),
			neuron.GetUpdateTime().AsTime().Format(time.RFC3339), state, neuron.GetName()}
		tableNeuron := pterm.TableData{header}

		tableNeuron = append(tableNeuron, row)

		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableNeuron).Render()
		if err != nil {
			return
		}

		// Display links to compare repos:
		if len(listNeuronVersionsRes.GetNeuronVersions()) > 1 {
			pterm.Printf("Recent changes:\n")
			pterm.Printf("Product: https://source.cloud.google.com/%s/product.%s/+/%s...%s\n", organisation.GetGoogleProjectId(), productID, listNeuronVersionsRes.GetNeuronVersions()[0].GetCommitSha(), listNeuronVersionsRes.GetNeuronVersions()[1].GetCommitSha())
			pterm.Printf("Proto: https://source.cloud.google.com/%s/proto/+/%s...%s\n\n", organisation.GetGoogleProjectId(), listNeuronVersionsRes.GetNeuronVersions()[0].GetProtoCommitSha(), listNeuronVersionsRes.GetNeuronVersions()[1].GetProtoCommitSha())
		}

		// Default envs
		table := pterm.TableData{[]string{"Env", "Default Value"}}
		for _, e := range neuron.GetEnvs() {
			table = append(table, []string{e.GetName(), e.GetValue()})
		}
		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
		if err != nil {
			return
		}

		// Display table of the last 7 neuron_versions
		pterm.DefaultSection.Print("NEURON BUILD HISTORY (last 7):")
		header = []string{"Index", "Version", "State", "Update Time", "Repositories"}
		neuronVersionTable := pterm.TableData{header}
		for i, neuronVersion := range neuronVersions {
			if i > 7 {
				continue
			}
			neuronVersionTable = append(neuronVersionTable, []string{
				fmt.Sprintf("%v", i),
				neuronVersion.GetVersion(),
				neuronVersion.GetState().String(),
				neuronVersion.GetUpdateTime().AsTime().Format(time.RFC3339),
				pterm.Gray(fmt.Sprintf("product: https://source.cloud.google.com/%s/product.%s/+/%s", organisation.GetGoogleProjectId(), productID, neuronVersion.GetCommitSha())),
			})
			neuronVersionTable = append(neuronVersionTable, []string{
				"",
				"",
				"",
				"",
				pterm.Gray(fmt.Sprintf("proto:   https://source.cloud.google.com/%s/proto/+/%s", organisation.GetGoogleProjectId(), neuronVersion.GetProtoCommitSha())),
			})
		}
		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(neuronVersionTable).Render()
		if err != nil {
			return
		}

		// Generate table with Deployment details.
		pterm.DefaultSection.Print("NEURON DEPLOYMENTS:")
		header = []string{"Index", "Name", "Neuron Version", "Google Project", "State", "Update Time"}
		deploymentTable := pterm.TableData{header}

		// Add deployments
		var neuronDeploymentNames []string
		for _, productDeployment := range productDeployments {
			neuronDeploymentNames = append(neuronDeploymentNames, productDeployment.GetName()+"/neurons/"+neuronID)
		}

		batchGetNeuronDeploymentsRes, err := alisProductsClient.BatchGetNeuronDeployments(cmd.Context(),
			&pbProducts.BatchGetNeuronDeploymentsRequest{
				Names: neuronDeploymentNames,
			})

		allEnvs := map[string]string{} // keep track of all env across all deployments
		for i, neuronDeployment := range batchGetNeuronDeploymentsRes.GetNeuronDeployments() {
			// only return valid deployments
			if neuronDeployment.GetName() != "" {

				version := neuronDeployment.GetVersion()
				if version != neuronVersion.GetVersion() {
					version += pterm.LightYellow("*")
				}
				row := []string{
					fmt.Sprintf("%v", i),
					productDeployments[i].GetDisplayName(),
					version,
					productDeployments[i].GetGoogleProjectId(),
					productDeployments[i].GetState().String(),
					productDeployments[i].GetUpdateTime().AsTime().Format(time.RFC3339),
				}

				// create env map for deployment.
				for _, e := range neuronDeployment.GetEnvs() {
					allEnvs[e.GetName()] = ""
				}

				deploymentTable = append(deploymentTable, row)
			}
		}

		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(deploymentTable).Render()
		if err != nil {
			return
		}

		// build header for Envs table.
		header = []string{"Env"}
		for i, neuronDeployment := range batchGetNeuronDeploymentsRes.GetNeuronDeployments() {
			if neuronDeployment.GetName() != "" {
				header = append(header, fmt.Sprintf("%v: %s", i, productDeployments[i].GetGoogleProjectId()))
			}
		}
		table = pterm.TableData{header}

		for env, _ := range allEnvs {
			row := []string{env}
			for _, neuronDeployment := range batchGetNeuronDeploymentsRes.GetNeuronDeployments() {
				if neuronDeployment.GetName() != "" {
					envMap := map[string]string{}
					for _, env := range neuronDeployment.GetEnvs() {
						envMap[env.GetName()] = env.GetValue()
					}
					row = append(row, envMap[env])
				}
			}
			table = append(table, row)
		}

		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
		if err != nil {
			return
		}
	},
}

// listNeuronCmd represents the list command
var listNeuronCmd = &cobra.Command{
	Use:     "list",
	Short:   pterm.Blue("Lists the neurons for a specified product"),
	Example: pterm.LightYellow("alis neuron list {orgID}.{productID}"),
	Args:    validateProductArg,
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the product resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetProduct:\n%s\n", product)

		// Retrieve the neuron resource
		listNeuronsRes, err := alisProductsClient.ListNeurons(cmd.Context(),
			&pbProducts.ListNeuronsRequest{Parent: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("ListNeurons:\n%v found\n", len(listNeuronsRes.GetNeurons()))

		productsDeploymentsRes, err := alisProductsClient.ListProductDeployments(cmd.Context(), &pbProducts.ListProductDeploymentsRequest{
			Parent: product.GetName(),
		})
		productDeployments := productsDeploymentsRes.GetProductDeployments()
		pterm.Debug.Printf("GetProductDeployments:\n%v found\n", len(productsDeploymentsRes.GetProductDeployments()))

		pterm.DefaultSection.Printf("Neurons for %s (%s):", product.GetDisplayName(), product.GetGoogleProjectId())

		table := pterm.TableData{{"Index", "Neuron ID", "Version", "Update Time", "State", "Resource Name"}}
		for i, neuron := range listNeuronsRes.GetNeurons() {
			// Retrieve the latest neuronVersion
			listNeuronVersionsRes, err := alisProductsClient.ListNeuronVersions(cmd.Context(), &pbProducts.ListNeuronVersionsRequest{
				Parent: neuron.GetName(),
			})
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			neuronVersions := listNeuronVersionsRes.GetNeuronVersions()

			var neuronVersion *pbProducts.NeuronVersion
			if len(neuronVersions) > 0 {
				neuronVersion = listNeuronVersionsRes.GetNeuronVersions()[0]
			}

			// Color the state
			state := neuronVersion.GetState().String()
			switch neuronVersion.GetState() {
			case pbProducts.NeuronVersion_FAILED:
				state = pterm.Red(state)
			}

			resourceID := strings.Split(neuron.GetName(), "/")[5]
			table = append(table, []string{
				strconv.Itoa(i), resourceID, neuronVersion.GetVersion(),
				neuron.GetUpdateTime().AsTime().Format(time.RFC3339), state, neuron.GetName()})

			// Add deployments
			var neuronDeploymentNames []string
			for _, productDeployment := range productDeployments {
				neuronDeploymentNames = append(neuronDeploymentNames, productDeployment.GetName()+"/neurons/"+resourceID)
			}

			batchGetNeuronDeploymentsRes, err := alisProductsClient.BatchGetNeuronDeployments(cmd.Context(),
				&pbProducts.BatchGetNeuronDeploymentsRequest{
					Names: neuronDeploymentNames,
				})

			for i, neuronDeployment := range batchGetNeuronDeploymentsRes.GetNeuronDeployments() {
				if neuronDeployment.GetName() != "" {

					version := neuronDeployment.GetVersion()
					if version != neuronVersion.GetVersion() {
						version += pterm.LightYellow("*")
					}
					table = append(table, []string{
						"", pterm.Gray(productDeployments[i].GetDisplayName()), pterm.Gray(version),
						pterm.Gray(productDeployments[i].GetGoogleProjectId())})
				}
			}
		}

		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
		if err != nil {
			return
		}
	},
}

// buildNeuronCmd represents the build command
var buildNeuronCmd = &cobra.Command{
	Use:   "build",
	Short: pterm.Blue("Builds a new version of the Neuron"),
	Long: pterm.Green(
		`This method retrieves the current version of the neuron and increments it in line 
with semantic versioning.  This also ensures that the neuron is inline with its infrastructure
specification as defined in your proto.

The neuron artifacts will be generated and pushed to the product artifact registry.  
This registry then becomes the source for neuron deployments.`),
	Example: pterm.LightYellow("alis neuron build {orgID}.{productID}.{neuronID}\nalis neuron build alis.in.resources-events-v1"),
	Args:    validateNeuronArg,
	Run: func(cmd *cobra.Command, args []string) {

		var commitSha string
		var protoCommitSha string

		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]
		neuronID = strings.Split(args[0], ".")[2]

		// fail the build if there is a `replace` entry in the go.mod file.
		neuronPath := fmt.Sprintf("%s/alis.exchange/%s/products/%s/%s", homeDir, organisationID, productID, strings.ReplaceAll(neuronID, "-", "/"))
		goMod, err := getGoMod(cmd.Context(), neuronPath)
		// don't fail when err != nil - i.e. there is not goMod file.
		if err == nil && goMod.Replace != nil {
			pterm.Error.Printf("When building a new NeuronVersion, `replace` entries are not allowed in your go.mod (%s/go.mod) file\nPlease remove / comment out the following before running `alis neuron build %s.%s.%s`\n", neuronPath, organisationID, productID, neuronID)
			for _, e := range goMod.Replace {
				pterm.Printf(" %s %s 👉 %s\n", pterm.Red("●"), e.Old.Path, e.New.Path)
			}
			return
		}

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the product resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetProduct:\n%s\n", product)

		// Retrieve the neuron resource
		neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
			&pbProducts.GetNeuronRequest{
				Name: "organisations/" + organisationID + "/products/" + productID + "/neurons/" + neuronID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetNeuron:\n%s\n", neuron)

		// generate a FileDescriptorSet from the current protos.
		// TODO: move this potentially to Build Triggers.
		fds, err := getNeuronDescriptor(neuron.GetName())
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// Retrieve the latest version
		res, err := alisProductsClient.ListNeuronVersions(cmd.Context(), &pbProducts.ListNeuronVersionsRequest{
			Parent:   neuron.GetName(),
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"version"}},
		})
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// Retrieve the latest version
		var latestVersion string
		var newVersion string
		if len(res.GetNeuronVersions()) > 0 {
			latestVersion = res.GetNeuronVersions()[0].GetVersion()
			newVersion, err = bumpVersion(latestVersion, releaseType)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			pterm.Info.Printf("Updating from version " + latestVersion + " to version " + newVersion + "...\n")
		} else {
			majorVersion := strings.Split(neuronID, "-")[2][1:]
			newVersion = majorVersion + ".0.0"
			pterm.Info.Printf("Creating initial version " + newVersion + "...\n")
		}

		// Tag the product and proto repositories with the newVersion
		for {
			rnd := generateRandomId(7)
			tag := fmt.Sprintf("%s.%s.%s.%s.%s", organisationID, productID, neuronID, newVersion, rnd)

			// tag product repository
			repoPath := fmt.Sprintf("%s/alis.exchange/%s/products/%s", homeDir, organisationID, productID)
			commitPath := fmt.Sprintf("%s/alis.exchange/%s/products/%s/%s", homeDir, organisationID, productID, strings.ReplaceAll(neuronID, "-", "/"))
			message := fmt.Sprintf("update(%s.%s.%s): %s", organisationID, productID, neuronID, newVersion)
			commitSha, err = commitTagAndPush(cmd.Context(), repoPath, commitPath, message, tag, false, false)
			// handle the case when the version already exists
			// ask whether the user would like to bump to the next version
			if status.Code(err) == codes.AlreadyExists {
				newVersion, err = bumpVersion(newVersion, "patch")
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				input, err := askUserString(fmt.Sprintf("Bump to version %s and continue (y|n)?: ", newVersion), `^[y|n]$`)
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				if input == "y" {
					tag = fmt.Sprintf("%s.%s.%s.%s", organisationID, productID, neuronID, newVersion)
					commitSha, err = commitTagAndPush(cmd.Context(), repoPath, commitPath, message, tag, false, false)
					break
				} else {
					pterm.Warning.Println("Aborting operation")
					return
				}
			}
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			// tag proto repository
			repoPath = fmt.Sprintf("%s/alis.exchange/%s/proto", homeDir, organisationID)
			commitPath = fmt.Sprintf("%s/alis.exchange/%s/proto/%s/%s/%s", homeDir, organisationID, organisationID, productID, strings.ReplaceAll(neuronID, "-", "/"))
			message = fmt.Sprintf("update(%s.%s.%s): %s", organisationID, productID, neuronID, newVersion)
			protoCommitSha, err = commitTagAndPush(cmd.Context(), repoPath, commitPath, message, tag, true, false)
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			break
		}

		// request Env updates from user.
		envs := neuron.GetEnvs()
		if setUpdateNeuronEnvFlag {
			envs, err = askUserNeuronEnvs(envs)
		}

		// retrieve available Dockerfiles
		neuronArg := fmt.Sprintf("%s.%s.%s", organisationID, productID, strings.ReplaceAll(neuronID, "-", "."))
		dockerFilePaths, err := findNeuronDockerFilePaths(neuronArg)
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Info.Printf("Found %v Dockerfile(s) in the neuron.\n", len(dockerFilePaths))

		// Create a new neuron
		op, err := alisProductsClient.CreateNeuronVersion(cmd.Context(), &pbProducts.CreateNeuronVersionRequest{
			Parent: neuron.GetName(),
			NeuronVersion: &pbProducts.NeuronVersion{
				CommitSha:         commitSha,
				ProtoCommitSha:    protoCommitSha,
				DockerfilePaths:   dockerFilePaths,
				FileDescriptorSet: fds,
			},
			NeuronVersionId: newVersion,
		})
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// check if we need to wait for operation to complete.
		if asyncFlag {
			pterm.Debug.Printf("GetOperation:\n%s\n", op)
			pterm.Success.Printf("Launched Update in async mode.\n see long-running operation " + op.GetName() + " to monitor state\n")
		} else {
			// wait for the long-running operation to complete.
			err := wait(cmd.Context(), op, "Updating "+neuron.GetName(), "Updated "+neuron.GetName(), 300, true)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
		}
	},
}

// deployNeuronCmd represents the deploy command
var deployNeuronCmd = &cobra.Command{
	Use:   "deploy",
	Short: pterm.Blue("Deploy a specified neuron to a chosen environment"),
	Long: pterm.Green(
		`This method retrieves the latest version of the neuron and
deploys it to one or more environments`),
	Args:    validateNeuronArg,
	Example: pterm.LightYellow("alis neuron deploy {orgID}.{productID}.{neuronID}\nalis neuron deploy alis.in.resources-events-v1"),
	Run: func(cmd *cobra.Command, args []string) {
		var op *longrunning.Operation
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]
		neuronID = strings.Split(args[0], ".")[2]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the product resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetProduct:\n%s\n", product)

		// Retrieve the neuron resource
		neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
			&pbProducts.GetNeuronRequest{
				Name: "organisations/" + organisationID + "/products/" + productID + "/neurons/" + neuronID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetNeuron:\n%s\n", neuron)

		// ask the user to select a product deployment
		productDeployments, err := selectProductDeployments(cmd.Context(), product.GetName())
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// Retrieve the latest version
		res, err := alisProductsClient.ListNeuronVersions(cmd.Context(), &pbProducts.ListNeuronVersionsRequest{
			Parent:   neuron.GetName(),
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"version"}},
		})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		if len(res.GetNeuronVersions()) == 0 {
			pterm.Error.Println("there are no versions available, please run `alis neron build ...` to create a version")
			return
		}

		latestVersion := res.GetNeuronVersions()[0].GetVersion()

		for _, productDeployment := range productDeployments {
			pterm.DefaultSection.Printf("Deploying %s (%s)", productDeployment.GetDisplayName(), productDeployment.GetGoogleProjectId())
			neuronDeployment, err := alisProductsClient.GetNeuronDeployment(cmd.Context(),
				&pbProducts.GetNeuronDeploymentRequest{
					Name: productDeployment.GetName() + "/neurons/" + neuronID})
			if status.Code(err) == codes.NotFound {
				pterm.Warning.Printf("This neuron has not yet been deployed to %s (%s)\n",
					productDeployment.GetDisplayName(), productDeployment.GetGoogleProjectId())

				input, err := askUserString("Would you like to create a new NeuronDeployment resource? (y|n): ", `^[y|n]$`)
				if input == "n" {
					pterm.Warning.Printf("selected 'n', aborting operation.")
					return
				}

				// set envs
				envs := neuron.GetEnvs()
				envs, err = askUserNeuronEnvs(envs)

				// Create a new NeuronDeployment resource
				op, err = alisProductsClient.CreateNeuronDeployment(cmd.Context(), &pbProducts.CreateNeuronDeploymentRequest{
					Parent: productDeployment.GetName(),
					NeuronDeployment: &pbProducts.NeuronDeployment{
						Envs:    envs,
						Version: latestVersion,
					},
					NeuronDeploymentId: neuronID,
				})
				if err != nil {
					pterm.Error.Println(err)
					return
				}
			} else if err != nil {
				pterm.Error.Println(err)
				return
			} else if setDeployNeuronStateFlag {
				// Updating the state of the deployment
				state, err := askUserNeuronDeploymentState(neuronDeployment.GetState())
				op, err = alisProductsClient.UpdateNeuronDeployment(cmd.Context(), &pbProducts.UpdateNeuronDeploymentRequest{
					NeuronDeployment: &pbProducts.NeuronDeployment{
						Name:  neuronDeployment.GetName(),
						State: state,
					},
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"state"},
					},
				})
				if err != nil {
					pterm.Error.Println(err)
					return
				}
			} else {
				// Update envs if '-e' flag was set.
				envs := neuronDeployment.GetEnvs()
				if setNeuronDeploymentEnvFlag {
					envs, err = askUserNeuronEnvs(neuronDeployment.GetEnvs())
				}

				pterm.Info.Printf("Updating deployment: %s | v%s ...\n",
					productDeployment.GetGoogleProjectId(), latestVersion)

				op, err = alisProductsClient.UpdateNeuronDeployment(cmd.Context(), &pbProducts.UpdateNeuronDeploymentRequest{
					NeuronDeployment: &pbProducts.NeuronDeployment{
						Name:    neuronDeployment.GetName(),
						Version: latestVersion,
						Envs:    envs,
					},
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"version", "envs"},
					},
				})
				if err != nil {
					pterm.Error.Println(err)
					return
				}
			}

			// check if we need to wait for operation to complete.
			if asyncFlag {
				pterm.Debug.Printf("GetOperation:\n%s\n", op)
				pterm.Success.Printf("Launched service in async mode.\n see long-running operation " + op.GetName() + " to monitor state\n")
			} else {
				// wait for the long-running operation to complete.
				err := wait(cmd.Context(), op, "Updating "+productDeployment.GetName(), "Updated "+productDeployment.GetName(), 300, true)
				if err != nil {
					pterm.Error.Println(err)
					return
				}
			}

			//// show link to Rover Visualisation
			//// make another hit to the neuronDeployment to retrieve the updated infrastructure url.
			//neuronDeployment, err = alisProductsClient.GetNeuronDeployment(cmd.Context(),
			//	&pbProducts.GetNeuronDeploymentRequest{
			//		Name: productDeployment.GetName() + "/neurons/" + neuronID})
			//pterm.Info.Printf("Terraform Visualisation:\n%s\n", neuronDeployment.GetInfrastructureUri())
		}
	},
}

// genprotoNeuronCmd represents the genproto command
var genprotoNeuronCmd = &cobra.Command{
	Use:   "genproto",
	Short: pterm.Blue("Generates the protocol buffers files for a specified language.  Golang is the default."),
	Long: pterm.Green(
		`This method uses the 'genproto-go' command line to generate protocol buffers 
for the specified neuron.  These are generated locally only and should be used for local development of your gRPC services.

These are used in combination with the 'Replace ...' command in your go.mod file to 'point' to the local, instead of the
official protobufs.'

Once local development is done, use the '--push' flag to push the generated protocol buffers to the go.protobuf repository.
Once the protobufs are pushed, you should remove the 'Replace... ' command in your go.mod file and run 'go mod tidy' to pull
the latest protobufs from the repo into your gRPC service.`),
	Example: pterm.LightYellow("alis neuron genproto {orgID}.{productID}.{neuronID}"),
	Args:    validateNeuronArg,
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]
		neuronID = strings.Split(args[0], ".")[2]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			// TODO: handle not found by listing available organisations.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("Get Organisation:\n%s\n", organisation)

		// Retrieve the neuron resource
		neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
			&pbProducts.GetNeuronRequest{
				Name: "organisations/" + organisationID + "/products/" + productID + "/neurons/" + neuronID})
		if err != nil {
			// TODO: handle not found by listing available products.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("Get Neuron:\n%s\n", neuron)

		// Generate the protocol buffers for Golang
		if genprotoGo {
			// set required path variables
			neuronProtobufFullPath := homeDir + "/alis.exchange/" + organisationID + "/protobuf/go/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
			neuronProtoFullPath := homeDir + "/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
			protobufGoRepoPath := homeDir + "/alis.exchange/" + organisationID + "/protobuf/go"

			// Clear any uncommitted changes to the repository
			// This ensures that we are able to push protobuf changes generated in the next section in all scenarios
			// When working on multiple neurons at the same time, there could be other uncommitted changes which will
			// cause a merge conflict when committing the new protocol buffers in the push section below.
			if pushProtocolBuffers {
				cmds := "git -C " + protobufGoRepoPath + " reset --hard"
				pterm.Debug.Printf("Shell command:\n%s\n", cmds)
				out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
				if err != nil {
					pterm.Error.Printf(fmt.Sprintf("%s", out))
					pterm.Error.Println(err)
					return
				}
			}

			// Clear all files in the relevant neuron folder.
			// TODO: refactor the use of GOPRIVATE envs.
			cmds := "rm -rf " + neuronProtobufFullPath + " && " +
				"mkdir -p " + neuronProtobufFullPath + " && " +
				"go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev && " +
				"protoc --go_out=$HOME/alis.exchange/" + organisationID + "/protobuf/go --go_opt=paths=source_relative --go-grpc_out=$HOME/alis.exchange/" + organisationID + "/protobuf/go --go-grpc_opt=paths=source_relative -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto " + neuronProtoFullPath + "/*.proto"

			pterm.Debug.Printf("Shell command:\n%s\n", cmds)
			out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
			if err != nil {
				pterm.Error.Printf(fmt.Sprintf("%s", out))
				pterm.Error.Println(err)
				return
			}
			if strings.Contains(fmt.Sprintf("%s", out), "warning") {
				pterm.Warning.Print(fmt.Sprintf("Generating protocol buffers for go...\n%s", out))
			}
			pterm.Success.Printf("Generated protocol buffers for Go.\nProto source: %s\n", neuronProtoFullPath)

			// generate ProductDescriptorFile at product level.
			err = genProductDescriptorFile("organisations/" + organisationID + "/products/" + productID)
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			pterm.Success.Println("Generated Product Descriptor File")

			// Publish to protobuf repository if not in local mode.
			if pushProtocolBuffers {
				// commit protocol buffers in go
				message := fmt.Sprintf("chore(%s): updated by alis_ CLI", neuronID)
				_, err := commitTagAndPush(cmd.Context(), protobufGoRepoPath, neuronProtobufFullPath,
					message, "", true, true)
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				pterm.Success.Println("Published protocol buffers for Go")

				ptermTip.Printf("Now that your protobuf if updated, please ensure that you update your \n" +
					"go.mod file to reflect this new version of your protobuf.\n")
			} else {
				ptermTip.Printf("The protobufs were generated for local development use only. To formally\n" +
					"publish them use the `--push` flag to publish them to the \n" +
					"protobuf libraries.\n")
			}
		}

		// generate protocol buffers for Python
		if genprotoPython {
			neuronProtobufFullPath := homeDir + "/alis.exchange/" + organisationID + "/protobuf/python/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
			neuronProtoFullPath := homeDir + "/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")

			// TODO: create __init__.py using golang file io.
			cmds := "rm -rf " + neuronProtobufFullPath + " && " +
				"mkdir -p " + neuronProtobufFullPath + " && " +
				`echo "__import__('pkg_resources').declare_namespace(__name__)" > $HOME/alis.exchange/` + organisationID + `/protobuf/python/` + organisationID + `/` + productID + `/__init__.py` + " && " +
				`echo "__import__('pkg_resources').declare_namespace(__name__)" > $HOME/alis.exchange/` + organisationID + `/protobuf/python/` + organisationID + `/` + productID + `/` + strings.Split(neuronID, "-")[0] + `/__init__.py` + " && " +
				`echo "__import__('pkg_resources').declare_namespace(__name__)" > $HOME/alis.exchange/` + organisationID + `/protobuf/python/` + organisationID + `/` + productID + `/` + strings.Split(neuronID, "-")[0] + `/` + strings.Split(neuronID, "-")[1] + `/__init__.py` + " && " +
				`echo "__import__('pkg_resources').declare_namespace(__name__)" > $HOME/alis.exchange/` + organisationID + `/protobuf/python/` + organisationID + `/` + productID + `/` + strings.Split(neuronID, "-")[0] + `/` + strings.Split(neuronID, "-")[1] + `/` + strings.Split(neuronID, "-")[2] + `/__init__.py` + " && " +
				"python3 -m grpc_tools.protoc --python_out=$HOME/alis.exchange/" + organisationID + "/protobuf/python --grpc_python_out=$HOME/alis.exchange/" + organisationID + "/protobuf/python -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto " + neuronProtoFullPath + "/*.proto"
			pterm.Debug.Printf("Shell command:\n%s\n", cmds)
			out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
			if err != nil {
				pterm.Error.Printf(fmt.Sprintf("%s", out))
				pterm.Error.Println(err)
				return
			}
			if strings.Contains(fmt.Sprintf("%s", out), "warning") {
				pterm.Warning.Print(fmt.Sprintf("Generating protocol buffers for python...\n%s", out))
			}
			pterm.Success.Printf("Generated protocol buffers for Python.\nProto source: %s\n", neuronProtoFullPath)

			// Publish to protobuf repository if not in local mode.
			if pushProtocolBuffers {

				protobufPythonRepo := fmt.Sprintf("%s/alis.exchange/%s/protobuf/python", homeDir, organisationID)

				// bump setup.py version
				var tpl bytes.Buffer
				setupPyTemplate, err := template.ParseFiles(protobufPythonRepo + "/setup.py")
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				if err = setupPyTemplate.Execute(&tpl, struct{}{}); err != nil {
					pterm.Error.Println(err)
					return
				}
				rel := regexp.MustCompile("version=\"(.*)\",")
				versionComponents := strings.Split(rel.FindStringSubmatch(tpl.String())[1], ".")
				minorVersion, err := strconv.Atoi(versionComponents[2])
				versionComponents[2] = strconv.Itoa(minorVersion + 1)
				newVersion := rel.ReplaceAllString(tpl.String(), "version=\""+strings.Join(versionComponents, ".")+"\",")
				out, err = exec.CommandContext(context.Background(), "bash", "-c", "echo "+"'"+newVersion+"'"+" > "+protobufPythonRepo+"/setup.py").CombinedOutput()
				if err != nil {
					pterm.Error.Printf(fmt.Sprintf("%s", out))
					pterm.Error.Println(err)
					return
				}

				// publish Python package to artifact registry
				tpl = bytes.Buffer{}
				publishTemplate, err := TemplateFs.ReadFile("internal/cmd/neuron/python/publishPython.sh")
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				t, err := template.New("file").Parse(string(publishTemplate))
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				if err := t.Execute(&tpl, struct {
					OrgProjectID   string
					OrganisationID string
				}{
					OrgProjectID:   organisation.GetGoogleProjectId(),
					OrganisationID: organisationID,
				}); err != nil {
					pterm.Error.Println(err)
					return
				}

				out, err = exec.CommandContext(context.Background(), "bash", "-c", tpl.String()).CombinedOutput()
				if err != nil {
					pterm.Error.Printf(fmt.Sprintf("%s", out))
					pterm.Error.Println(err)
					return
				}

				if strings.Contains(fmt.Sprintf("%s", out), "warning") {
					pterm.Warning.Print(fmt.Sprintf("Publishing protocol buffers for python...\n%s", out))
				}

				message := fmt.Sprintf("chore(%s): updated by alis_ CLI", neuronID)
				commitPath := neuronProtobufFullPath +
					" " + protobufPythonRepo + "/setup.py" +
					" " + protobufPythonRepo + "/alis/" + productID + "/__init__.py" +
					" " + protobufPythonRepo + "/alis/" + productID + "/" + strings.Split(neuronID, "-")[0] + "/__init__.py" +
					" " + protobufPythonRepo + "/alis/" + productID + "/" + strings.Split(neuronID, "-")[0] + "/" + strings.Split(neuronID, "-")[1] + "/__init__.py"

				_, err = commitTagAndPush(cmd.Context(), protobufPythonRepo, commitPath, message,
					"", true, true)
				if err != nil {
					return
				}

				pterm.Success.Println("Published protocol buffers for Python")
			} else {
				ptermTip.Printf("The protobufs were generated for local development use only. To formally\n" +
					"publish them use the `--push` flag to publish them to the \n" +
					"protobuf libraries.\n")
			}
		}

		if genprotoCpp {
			neuronProtobufFullPath := homeDir + "/alis.exchange/" + organisationID + "/protobuf/cpp/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
			neuronProtoFullPath := homeDir + "/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")

			// Clear all files in the relevant neuron folder. .
			cmds := "rm -rf " + neuronProtobufFullPath + " && " +
				"mkdir -p " + neuronProtobufFullPath + " && " +
				"protoc --cpp_out=$HOME/alis.exchange/" + organisationID + "/protobuf/cpp -I=$HOME/alis.exchange/google/proto --proto_path=$HOME/alis.exchange/" + organisationID + "/proto " + neuronProtoFullPath + "/*.proto"
			pterm.Debug.Printf("Shell command:\n%s\n", cmds)
			out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
			if err != nil {
				pterm.Error.Printf(fmt.Sprintf("%s", out))
				pterm.Error.Println(err)
				return
			}
			if strings.Contains(fmt.Sprintf("%s", out), "warning") {
				pterm.Warning.Print(fmt.Sprintf("Generating protocol buffers for python...\n%s", out))
			}
			pterm.Success.Printf("Generated protocol buffers for C++.\nProto source: %s\n", neuronProtoFullPath)

			// Publish to protobuf repository if not in local mode.
			if pushProtocolBuffers {
				protobufCppRepo := fmt.Sprintf("%s/alis.exchange/%s/protobuf/cpp", homeDir, organisationID)

				// commit protocol buffers in go
				message := fmt.Sprintf("chore(%s): updated by alis_ CLI", neuronID)
				_, err := commitTagAndPush(cmd.Context(), protobufCppRepo, neuronProtobufFullPath,
					message, "", true, true)
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				pterm.Success.Println("Published protocol buffers for C++")
			} else {
				ptermTip.Printf("The protobufs were generated for local development use only. To formally\n" +
					"publish them use the `--push` flag to publish them to the \n" +
					"protobuf libraries.\n")
			}

		}

		return
	},
}

// genApiNeuronCmd represents the genapi command
var genApiNeuronCmd = &cobra.Command{
	Use:   "genapi",
	Short: pterm.Blue("Generates the api libraries in go."),
	Long: pterm.Green(
		`This method uses the 'gapi-go' command line to generate protocol buffers 
for the specified neuron`),
	Example: pterm.LightYellow("alis neuron genapi {orgID}.{productID}.{neuronID}"),
	Args:    validateNeuronArg,
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]
		neuronID = strings.Split(args[0], ".")[2]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			// TODO: handle not found by listing available organisations.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the neuron resource
		neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
			&pbProducts.GetNeuronRequest{
				Name: "organisations/" + organisationID + "/products/" + productID + "/neurons/" + neuronID})
		if err != nil {
			// TODO: handle not found by listing available products.
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetNeuron:\n%s\n", neuron)

		// Generate the api client libraries buffers.
		neuronAPIFullPath := homeDir + "/alis.exchange/" + organisationID + "/api/go/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
		neuronProtoFullPath := homeDir + "/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
		cmds := "rm -rf " + neuronAPIFullPath + " && " +
			"mkdir -p " + neuronAPIFullPath + " && " +
			"go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev &&" +
			"protoc --go_gapic_out=$HOME/alis.exchange/" + organisationID + "/api/go --go_gapic_opt='go-gapic-package=" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/") + ";" + strings.Split(neuronID, "-")[2] + "' -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto " + neuronProtoFullPath + "/*.proto"

		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
		if err != nil {
			pterm.Error.Printf(fmt.Sprintf("%s", out))
			pterm.Error.Println(err)
			return
		}
		if strings.Contains(fmt.Sprintf("%s", out), "warning") {
			pterm.Warning.Print(fmt.Sprintf("Generating protocol buffers...\n%s", out))
		} else {
			pterm.Debug.Print(fmt.Sprintf("%s\n", out))
		}

		// Publish to api libraries
		if publishApiFlag {
			// commit protocol buffers in go
			apiGoRepo := fmt.Sprintf("%s/alis.exchange/%s/api/go", homeDir, organisationID)
			message := fmt.Sprintf("chore(%s): updated by alis_ CLI", neuronID)
			_, err = commitTagAndPush(cmd.Context(), apiGoRepo, neuronAPIFullPath,
				message, "", true, true)
			if err != nil {
				return
			}
			ptermTip.Printf("Now that your protobuf if updated, please ensure that you update your \n" +
				"go.mod file to reflect this new version of your protobuf.\n")
		} else {
			ptermTip.Printf("The protobufs were generated for local development use only. To formally\n" +
				"publish them use the `-p` or `--publish` flag to publish them to the \n" +
				"protobuf libraries.\n")
		}
		return
	},
}

func init() {
	rootCmd.AddCommand(neuronCmd)
	neuronCmd.AddCommand(createNeuronCmd)
	neuronCmd.AddCommand(getNeuronCmd)
	neuronCmd.AddCommand(listNeuronCmd)
	neuronCmd.AddCommand(buildNeuronCmd)
	neuronCmd.AddCommand(deployNeuronCmd)
	neuronCmd.AddCommand(genprotoNeuronCmd)
	neuronCmd.AddCommand(genApiNeuronCmd)
	neuronCmd.SilenceUsage = true
	neuronCmd.SilenceErrors = true

	deployNeuronCmd.Flags().BoolVarP(&setNeuronDeploymentEnvFlag, "env", "e", false, pterm.Green("Set or update the ENV variables."))
	deployNeuronCmd.Flags().BoolVarP(&setDeployNeuronStateFlag, "state", "s", false, pterm.Green("Update the state of the neuron.."))

	buildNeuronCmd.Flags().BoolVarP(&setUpdateNeuronEnvFlag, "env", "e", false, pterm.Green("Set or update the ENV variables."))
	buildNeuronCmd.Flags().BoolVarP(&setUpdateNeuronStateFlag, "state", "s", false, pterm.Green("Update the state of the neuron."))

	genprotoNeuronCmd.Flags().BoolVarP(&pushProtocolBuffers, "publish", "p", false, pterm.Green("Generate the protocol buffers and push them to the protobuf repository"))

	// Proto Generators
	genprotoNeuronCmd.Flags().BoolVarP(&genprotoGo, "go", "", true, pterm.Green("Generate the protocol buffers for Golang"))
	genprotoNeuronCmd.Flags().BoolVarP(&genprotoPython, "python", "", false, pterm.Green("Generate the protocol buffers for Python"))
	genprotoNeuronCmd.Flags().BoolVarP(&genprotoCpp, "cpp", "", false, pterm.Green("Generate the protocol buffers for C++"))
	//genApiNeuronCmd.Flags().BoolVarP(&publishApiFlag, "push", "p", false, pterm.Green("Generate the api libraries and push them to the api repository"))
}
