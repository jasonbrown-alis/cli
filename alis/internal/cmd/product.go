package cmd

import (
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	pbProducts "go.protobuf.alis.alis.exchange/alis/os/resources/products/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var (
	initProductFlag         bool
	getProductFlag          bool
	clearProductFlag        bool
	getProductKeyFlag       bool
	deployProductFlag       bool
	buildProductFlag        bool
	createNewDeploymentFlag bool
	setDeployProductEnvFlag bool
)

// productCmd represents the product command
var productCmd = &cobra.Command{
	Use:   "product",
	Short: pterm.Blue("Manages products within your organisation."),
	Long:  pterm.Green("Use this command to manage products within your organisation."),
	Run: func(cmd *cobra.Command, args []string) {
		pterm.Error.Println("a valid command is missing\nplease run 'alis product -h' for details.")
	},
}

// createProductCmd represents the create command
var createProductCmd = &cobra.Command{
	Use:   "create",
	Short: pterm.Blue("Creates a new product"),
	Long: pterm.Green(
		`This method creates a new product in the specified organisation`),
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

		// ensure that the product does not yet exist
		// we perform the check here before asking the user a range of questions - i.e. fail fast ;)
		_, err = alisProductsClient.GetProduct(cmd.Context(), &pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err == nil {
			// the resource exists
			pterm.Error.Printf("the product (%s.%s) already exist.\n", organisationID, productID)
			return
		}

		// Get additional user input
		displayName, err := askUserString("Please provide a Display Name: ", `^[A-Za-z0-9- ]+$`)
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		owner, err := askUserString("Please provide an owner (email): ", `(?m)^([a-zA-Z0-9_\-\.]+)@([a-zA-Z0-9_\-\.]+)\.([a-zA-Z]{2,10})$`)
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		description, err := askUserString("Describe the product: ", `^[A-Za-z0-9- .,_]+$`)
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		ptermTip.Println("The organisation has a billing account ID of " + strings.Split(organisation.GetBillingAccount(), "/")[1] + "\nNavigate to https://console.cloud.google.com/billing to see the billing accounts available to you.")
		billingAccountID, err := askUserString("Product level Billing Account ID: ", `^[A-Z0-9]{6}-[A-Z0-9]{6}-[A-Z0-9]{6}$`)
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// Get product Template files.
		// push boiler plate code to local environment
		files, err := TemplateFs.ReadDir("templates/product")
		if err != nil {
			return
		}
		for i, f := range files {

			fileTemplate, err := TemplateFs.ReadFile("templates/product/" + f.Name())
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

			destDir := fmt.Sprintf("%s/alis.exchange/%s/proto/%s/%s", homeDir, organisationID, organisationID, productID)
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
			p := Parameters{}
			err = t.Execute(file, p)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			pterm.Info.Printf("Created %s/%s\n", destDir, filename)
		}
		pterm.Warning.Printf("The above files have been added to your proto repository.\n" +
			"but have not yet been committed.\n" +
			"Make the necessary changes to the files, commit them before running the `alis product build` " +
			"command.\n")
		ptermTip.Printf("You'll need to get a copy of this new product to your local environment.\n"+
			"Run the command `alis product get %s.%s`", organisationID, productID)

		// Create a product
		op, err := alisProductsClient.CreateProduct(cmd.Context(), &pbProducts.CreateProductRequest{
			Parent: organisation.GetName(),
			Product: &pbProducts.Product{
				DisplayName:    displayName,
				Owner:          owner,
				Description:    description,
				BillingAccount: "billingAccounts/" + billingAccountID,
			},
			ProductId: productID,
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
			err = wait(cmd.Context(), op, "Creating "+organisation.GetName()+"/products/"+productID+" (may take a few minutes)", "Created "+organisation.GetName()+"/products/"+productID, 300, true)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
		}

		// Get the product resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: organisation.GetName() + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// display some user instructions to perform once a new product has been created.
		ptermTip.Println("Now that you have a new product there are a few minor things you need to take care of:")
		pterm.Printf("👉 Your product has a new service account: alis-exchange@%s.iam.gserviceaccount.com. The following permissions are required:\n"+
			"a. Navigate to https://console.cloud.google.com/billing and give the Billing Account User role to relevant billing account you will be using for your ProductDeployments.\n   (the Product Service Account needs to be able to allocate Billing Accounts to any deployments)\n"+
			"b. Navigate to https://admin.google.com/ac/roles and assign the Groups Editor role to this service account. (the Product Service account needs to be able to create a group for each deployment)\n", product.GetGoogleProjectId())
		pterm.Printf("👉 Your product has an initial development deployment, Development 001.")
		pterm.Printf("👉 Retrieve a copy of your repository using the command: " + pterm.LightYellow(fmt.Sprintf("alis product get %s.%s \n", organisationID, productID)))
		pterm.Println("👉 Open the repository in your IDE and create your first empty commit.")
	},
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product create foo.aa"),
}

// getProductCmd represents the get command
var getProductCmd = &cobra.Command{
	Use:   "get",
	Short: pterm.Blue("Retrieves a specified product"),
	Long: pterm.Green(
		`This method clones or updates the specified product repository to your local environment
under the 'alis.exchange' directory.`),
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product get {orgID}.{productID}"),
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

		// Clone the product repository
		spinner, _ := pterm.DefaultSpinner.Start("Updating " + homeDir + "/alis.exchange/" + organisationID + "/products/" + productID + "... ")
		cmds := "git -C $HOME/alis.exchange/" + organisationID + "/products/" + productID + " pull --no-rebase || gcloud source repos clone product." + productID + " $HOME/alis.exchange/" + organisationID + "/products/" + productID + " --project=" + organisation.GetGoogleProjectId()
		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err := exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
		if err != nil {
			pterm.Error.Printf(fmt.Sprintf("%s", out))
			return
		}
		if strings.Contains(string(out), "Already") {
			//fmt.Printf("\033[32m%s\033[0m", out)
		} else {
			pterm.Debug.Printf(fmt.Sprintf("%s", out))
		}
		spinner.Success("Updated " + homeDir + "/alis.exchange/" + organisationID + "/products/" + productID)
		ptermTip.Printf("Now that you have a local copy of the product, you may need to generate a key.\n" +
			"run `alis product getkey " + organisationID + "." + productID + "` to generate one.\n")
	},
}

// clearProductCmd represents the clear command
var clearProductCmd = &cobra.Command{
	Use:   "clear",
	Short: pterm.Blue("Clears the product from your local environment"),
	Long: pterm.Green(
		`This method removes the specified product from your local environment. 
Please clear products not actively working on - its not great to leave these lying 
around in your local development environment.`),
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]

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

		productPath := homeDir + "/alis.exchange/" + organisationID + "/products/" + productID
		pterm.Warning.Printf("Removing product '%s.%s' from your local environment.\nFolder location: %s\nPlease also ensure you close this product in any IDEs you may have open.\n", organisationID, productID, productPath)
		userInput, err := askUserString("Are you sure? (y/n): ", `^[y|n]$`)
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		if userInput == "y" {
			cmds := "rm -rf " + productPath
			pterm.Debug.Printf("Shell command:\n%s\n", cmds)
			out, err := exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
			if err != nil {
				pterm.Error.Printf(fmt.Sprintf("%s", out))
				return
			}
			pterm.Debug.Printf(fmt.Sprintf("%s", out))
			pterm.Success.Printf("Removed product `%s.%s` from your local environment.\nFolder removed: %s\n", organisationID, productID, productPath)
		} else {
			pterm.Warning.Printf("Aborted operation.\n Did not remove %s\n", productPath)
		}

	},
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product clear {orgID}.{productID}"),
}

// listProductCmd represents the list command
var listProductCmd = &cobra.Command{
	Use:   "list",
	Short: pterm.Blue("Lists all products in a given organisation"),
	//Long: pterm.Green(
	//	`This method lists all the products for a given organisation`),
	Args:    validateOrgArg,
	Example: pterm.LightYellow("alis product list {orgID}"),
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]

		// Retrieve the organisation resource
		organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
			&pbProducts.GetOrganisationRequest{Name: "organisations/" + organisationID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetOrganisation:\n%s\n", organisation)

		// Retrieve the product resource
		products, err := alisProductsClient.ListProducts(cmd.Context(),
			&pbProducts.ListProductsRequest{
				Parent: "organisations/" + organisationID,
			})
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		table := pterm.TableData{{"Index", "Product ID", "Display Name", "Version", "Owner", "Google Project", "Resource Name"}}
		for i, product := range products.GetProducts() {
			resourceID := strings.Split(product.GetName(), "/")[3]
			table = append(table, []string{
				strconv.Itoa(i), resourceID, product.GetDisplayName(), product.GetVersion(),
				product.GetOwner(), product.GetGoogleProjectId(), product.GetName()})
		}

		err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
		if err != nil {
			return
		}

	},
}

// treeProductCmd represents the tree command
var treeProductCmd = &cobra.Command{
	Use:   "tree",
	Short: pterm.Blue("Show a tree diagram of the product, its neurons and deployments"),
	//Long: pterm.Green(
	//	`This method lists all the products for a given organisation`),
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product tree {orgID}.{productID}"),
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]

		// Retrieve the organisation resource
		product, err := alisProductsClient.GetProduct(cmd.Context(),
			&pbProducts.GetProductRequest{Name: "organisations/" + organisationID + "/products/" + productID})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		pterm.Debug.Printf("GetProduct:\n%s\n", product)

		tree := pterm.LeveledList{}
		tree = append(tree, pterm.LeveledListItem{Level: 0, Text: "Products:"})

		productEntry := fmt.Sprintf("%s - %s | %s | %s | %s", strings.ToUpper(productID), product.GetDisplayName(), product.GetVersion(), product.GetState(), product.GetOwner())
		switch product.GetState() {
		case pbProducts.Product_FAILED:
			productEntry = pterm.BgRed.Sprint(productEntry)
		case pbProducts.Product_ACTIVE:
			productEntry = pterm.BgBlue.Sprint(productEntry)
		case pbProducts.Product_CREATING:
			productEntry = pterm.BgGray.Sprint(productEntry)
		case pbProducts.Product_UPDATING:
			productEntry = pterm.BgYellow.Sprint(productEntry)
		}

		tree = append(tree, pterm.LeveledListItem{Level: 1, Text: productEntry})

		// append Neurons
		neurons, err := alisProductsClient.ListNeurons(cmd.Context(), &pbProducts.ListNeuronsRequest{
			Parent: product.GetName(),
		})

		// create a version lookup map to determine whether a deployed neuron version is out dated.
		neuronVersionMap := map[string]string{}
		tree = append(tree, pterm.LeveledListItem{Level: 2, Text: pterm.Gray("Neurons:")})
		for i, neuron := range neurons.GetNeurons() {

			// Retrieve the latest version
			res, err := alisProductsClient.ListNeuronVersions(cmd.Context(), &pbProducts.ListNeuronVersionsRequest{
				Parent: neuron.GetName(),
			})
			if err != nil {
				pterm.Error.Println(err)
				return
			}

			var neuronVersion *pbProducts.NeuronVersion
			if len(res.GetNeuronVersions()) > 0 {
				neuronVersion = res.GetNeuronVersions()[0]
			}

			neuronVersionMap[strings.Split(neuron.GetName(), "/")[5]] = neuronVersion.GetVersion()
			neuronEntry := fmt.Sprintf("%v: %25s | %6s | %8s | %s", i, strings.Split(neuron.GetName(), "/")[5], neuronVersion.GetVersion(), neuronVersion.GetState(), neuronVersion.GetUpdateTime().AsTime().Format(time.RFC822))
			switch neuron.GetState() {
			case pbProducts.Neuron_FAILED:
				neuronEntry = pterm.Red(neuronEntry)
			case pbProducts.Neuron_ACTIVE:
				neuronEntry = pterm.Blue(neuronEntry)
			case pbProducts.Neuron_CREATING:
				neuronEntry = pterm.Gray(neuronEntry)
			case pbProducts.Neuron_UPDATING:
				neuronEntry = pterm.Yellow(neuronEntry)
			}
			// Add environment variables
			neuronEntry += pterm.Gray(fmt.Sprintf(" | %s", pterm.Gray(neuron.GetEnvs())))

			tree = append(tree, pterm.LeveledListItem{Level: 3, Text: neuronEntry})
		}
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		// append Deployments
		tree = append(tree, pterm.LeveledListItem{Level: 2, Text: pterm.Gray("Deployed Products:")})
		productDeployments, err := alisProductsClient.ListProductDeployments(cmd.Context(), &pbProducts.ListProductDeploymentsRequest{Parent: product.GetName()})
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		for i, productDeployment := range productDeployments.GetProductDeployments() {
			productDeploymentEntry := fmt.Sprintf("%v: %s | %s | %s | %s | %s | %s | %s", i, productDeployment.GetDisplayName(), productDeployment.GetGoogleProjectId(), productDeployment.GetVersion(), productDeployment.GetState(), productDeployment.GetUpdateTime().AsTime().Format(time.RFC822), productDeployment.GetOwner(), strings.Split(productDeployment.GetName(), "/")[5])
			switch productDeployment.GetState() {
			case pbProducts.ProductDeployment_FAILED:
				productDeploymentEntry = pterm.Red(productDeploymentEntry)
			case pbProducts.ProductDeployment_RUNNING:
				productDeploymentEntry = pterm.BgGreen.Sprint(productDeploymentEntry)
			case pbProducts.ProductDeployment_CREATING:
				productDeploymentEntry = pterm.BgGray.Sprint(productDeploymentEntry)
			case pbProducts.ProductDeployment_UPDATING:
				productDeploymentEntry = pterm.BgYellow.Sprint(productDeploymentEntry)
			case pbProducts.ProductDeployment_LOCKED:
				productDeploymentEntry = pterm.BgCyan.Sprint(productDeploymentEntry)
			}

			tree = append(tree, pterm.LeveledListItem{Level: 3, Text: productDeploymentEntry})
			// Add neurons to deployment
			//tree = append(tree, pterm.LeveledListItem{Level: 4, Text: pterm.Gray("Deployed Neurons:")})
			neuronDeployments, err := alisProductsClient.ListNeuronDeployments(cmd.Context(), &pbProducts.ListNeuronDeploymentsRequest{Parent: productDeployment.GetName()})
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			for i, neuronDeployment := range neuronDeployments.GetNeuronDeployments() {

				// Add an indicator to the version if it differs from the product level version.
				version := neuronDeployment.GetVersion()
				if neuronVersionMap[strings.Split(neuronDeployment.GetName(), "/")[7]] != neuronDeployment.GetVersion() {
					version += pterm.LightYellow("*")
				}

				neuronDeploymentEntry := fmt.Sprintf("%v: %25s | %7s | %8s | %s", i, strings.Split(neuronDeployment.GetName(), "/")[7], version, neuronDeployment.GetState(), neuronDeployment.GetUpdateTime().AsTime().Format(time.RFC822))
				switch neuronDeployment.GetState() {
				case pbProducts.NeuronDeployment_FAILED:
					neuronDeploymentEntry = pterm.Red(neuronDeploymentEntry)
				case pbProducts.NeuronDeployment_RUNNING:
					neuronDeploymentEntry = pterm.Green(neuronDeploymentEntry)
				case pbProducts.NeuronDeployment_CREATING:
					neuronDeploymentEntry = pterm.Gray(neuronDeploymentEntry)
				case pbProducts.NeuronDeployment_UPDATING:
					neuronDeploymentEntry = pterm.Yellow(neuronDeploymentEntry)
				}

				// Add environment variables
				neuronDeploymentEntry += pterm.Gray(fmt.Sprintf(" | %s", neuronDeployment.GetEnvs()))

				tree = append(tree, pterm.LeveledListItem{Level: 4, Text: neuronDeploymentEntry})
			}
		}

		root := pterm.NewTreeFromLeveledList(tree)
		err = pterm.DefaultTree.WithRoot(root).Render()
		if err != nil {
			pterm.Error.Println(err)
			return
		}
	},
}

// buildProductCmd represents the build command
var buildProductCmd = &cobra.Command{
	Use:   "build",
	Short: pterm.Blue("Updates the product to its next version"),
	Long: pterm.Green(
		`This method retrieves the current version of the product and increments it in line 
with semantic versioning.  This also ensures that the product is inline with its infrastructure
specification as determined by alis.exchange.`),
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]

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

		// TODO: handle the scenario where user provides a specific version.
		newVersion, err := bumpVersion(product.GetVersion(), releaseType)

		for {
			pterm.Info.Printf("Updating from version " + product.GetVersion() + " to version " + newVersion + "...\n")

			// tag product repository
			tag := fmt.Sprintf("%s.%s.%s", organisationID, productID, newVersion)
			repoPath := fmt.Sprintf("%s/alis.exchange/%s/products/%s", homeDir, organisationID, productID)
			commitPath := fmt.Sprintf("%s/alis.exchange/%s/products/%s", homeDir, organisationID, productID)
			message := fmt.Sprintf("update(%s.%s): %s", organisationID, productID, newVersion)
			_, err = commitTagAndPush(cmd.Context(), repoPath, commitPath, message, tag, false, false)
			// handle the case when the version already exists
			// ask whether the user would like to bump to the next version
			if status.Code(err) == codes.AlreadyExists {
				//pterm.Warning.Println(err)
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
			commitPath = fmt.Sprintf("%s/alis.exchange/%s/proto/%s/%s", homeDir, organisationID, organisationID, productID)
			message = fmt.Sprintf("update(%s.%s): %s", organisationID, productID, newVersion)
			_, err = commitTagAndPush(cmd.Context(), repoPath, commitPath, message, tag, false, false)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			break
		}

		// Updating Product
		//spinner, _ := pterm.DefaultSpinner.Start("Updating from version " + product.GetVersion() + " to version " + newVersion)

		op, err := alisProductsClient.UpdateProduct(cmd.Context(), &pbProducts.UpdateProductRequest{
			Product: &pbProducts.Product{
				Name:    "organisations/" + organisationID + "/products/" + productID,
				Version: newVersion,
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"version"},
			},
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
			err := wait(cmd.Context(), op, "Updating "+product.GetName(), "Updated "+product.GetName(), 300, true)
			if err != nil {
				pterm.Error.Println(err)
				return
			}
		}
	},
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product build {orgID}.{productID}"),
}

// deployProductCmd represents the get command
var deployProductCmd = &cobra.Command{
	Use:   "deploy",
	Short: pterm.Blue("Deploy the product to environment(s)"),
	Long: pterm.Green(
		`This method retrieves the latest version of the product and
deploys it to one or more environments`),
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product deploy {orgID}.{productID}"),
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

		// ask the user to select one or more deployments
		productDeployments, err := selectProductDeployments(cmd.Context(), product.GetName())
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		for _, productDeployment := range productDeployments {
			pterm.DefaultSection.Printf("Deploying %s (%s)", productDeployment.GetDisplayName(), productDeployment.GetGoogleProjectId())

			// don't update if the deployment already reflects the latest product version.
			if productDeployment.GetVersion() == product.GetVersion() {
				pterm.Warning.Printf("the deployment %s is running the latest product version of %s\n", productDeployment.GetGoogleProjectId(), product.GetVersion())
				input, err := askUserString("Still continue (y|n)?: ", `^[y|n]$`)
				if err != nil {
					pterm.Error.Println(err)
					return
				}
				if input == "n" {
					continue
				}
			}

			// Update envs if '-e' flag was set.
			envs := productDeployment.GetEnvs()
			if setDeployProductEnvFlag {
				envs, err = askUserProductEnvs(productDeployment.GetEnvs())
			}

			pterm.Info.Printf("Updating deployment: %s\nversion: %s -> %s...\n", productDeployment.GetGoogleProjectId(), productDeployment.GetVersion(), product.GetVersion())
			op, err := alisProductsClient.UpdateProductDeployment(cmd.Context(), &pbProducts.UpdateProductDeploymentRequest{
				ProductDeployment: &pbProducts.ProductDeployment{
					Name:    productDeployment.GetName(),
					Version: product.GetVersion(),
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

			// check if we need to wait for operation to complete.
			if asyncFlag {
				pterm.Debug.Printf("GetOperation:\n%s\n", op)
				pterm.Success.Printf("Launched Update in async mode.\n see long-running operation " + op.GetName() + " to monitor state\n")
			} else {
				// wait for the long-running operation to complete.
				err := wait(cmd.Context(), op, "Updating "+productDeployment.GetName(), "Updated "+productDeployment.GetName(), 300, true)
				if err != nil {
					pterm.Error.Println(err)
					return
				}
			}
			//// show link to Rover Visualisation
			//productDeployment, err = alisProductsClient.GetProductDeployment(cmd.Context(), &pbProducts.GetProductDeploymentRequest{Name: productDeployment.GetName()})
			//if err != nil {
			//	pterm.Error.Println(err)
			//	return
			//}
			//pterm.Info.Printf("Terraform Visualisation:\n%s\n", productDeployment.GetInfrastructureUri())
		}
	},
}

// getkeyProductCmd represents the getkey command
var getkeyProductCmd = &cobra.Command{
	Use:   "getkey",
	Short: pterm.Blue("Retrieves a service account key product"),
	Long: pterm.Green(
		`This method uses the gcloud command to create a key.`),
	Args:    validateProductArg,
	Example: pterm.LightYellow("alis product getkey {orgID}.{productID}"),
	Run: func(cmd *cobra.Command, args []string) {
		organisationID = strings.Split(args[0], ".")[0]
		productID = strings.Split(args[0], ".")[1]

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

		// ask the user to select a deployment
		productDeployments, err := selectProductDeployments(cmd.Context(), product.GetName())
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		for _, productDeployment := range productDeployments {
			// Generate a token
			spinner, _ := pterm.DefaultSpinner.Start("Generating token for " + productDeployment.GetGoogleProjectId() + "... ")
			cmds := "gcloud iam service-accounts keys create $HOME/alis.exchange/" + organisationID + "/products/" + productID +
				"/key-" + productDeployment.GetGoogleProjectId() + ".json --iam-account=alis-exchange@" +
				productDeployment.GetGoogleProjectId() + ".iam.gserviceaccount.com --project=" +
				productDeployment.GetGoogleProjectId()
			pterm.Debug.Printf("Shell command:\n%s\n", cmds)
			out, err := exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
			if err != nil {
				pterm.Debug.Println(cmds)
				spinner.Fail(fmt.Sprintf("%s", out))
				return
			}
			spinner.Success("Retrieved Token: alis-exchange@" + productDeployment.GetGoogleProjectId() + ".iam.gserviceaccount.com\nSaved at: " + homeDir + "/alis.exchange/" + organisationID + "/products/" + productID + "\n")
			ptermTip.Printf("In your IDE, ensure that you have the following environmental variable set:\n" +
				"GOOGLE_APPLICATION_CREDENTIALS=../../../key-" + productDeployment.GetGoogleProjectId() + ".json\n")
		}
		pterm.Warning.Println("as always don't leave these lying around ;)")

	},
}

// gendocsProductCmd represents the gendocs command
var gendocsProductCmd = &cobra.Command{
	Use:   "gendocs",
	Short: pterm.Blue("Generates documentation files for all proto files in the specified product."),
	Long: pterm.Green(
		`This method uses the 'genproto-go' command line to generate documentation for the
specified product.

It makes use of protoc-gen-doc plugin. Installation requirements:
https://github.com/pseudomuto/protoc-gen-doc#installation`),
	Example: pterm.LightYellow("alis product gendocs {orgID}.{productID}"),
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

		// Generate the index.html
		cmds := "go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev &&" +
			"protoc --plugin=protoc-gen-doc=$HOME/go/bin/protoc-gen-doc --doc_out=$HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " --doc_opt=html,docs.html -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto $(find $HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " -iname \"*.proto\")"
		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err := exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
		if err != nil {
			pterm.Error.Printf(fmt.Sprintf("%s", out))
			pterm.Error.Println(err)
			return
		}

		// Generate markdown
		cmds = "go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev &&" +
			"protoc --plugin=protoc-gen-doc=$HOME/go/bin/protoc-gen-doc --doc_out=$HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " --doc_opt=markdown,docs.md -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto $(find $HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " -iname \"*.proto\")"
		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err = exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
		if err != nil {
			pterm.Error.Printf(fmt.Sprintf("%s", out))
			pterm.Error.Println(err)
			return
		}
		// Generate json
		cmds = "go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev &&" +
			"protoc --plugin=protoc-gen-doc=$HOME/go/bin/protoc-gen-doc --doc_out=$HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " --doc_opt=json,docs.json -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto $(find $HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " -iname \"*.proto\")"
		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err = exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
		if err != nil {
			pterm.Error.Printf(fmt.Sprintf("%s", out))
			pterm.Error.Println(err)
			return
		}

		//// Generate openapi description
		//cmds = "go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev &&" +
		//	"protoc --openapi_out=$HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto $(find $HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " -iname \"*.proto\")"
		//out, err = exec.CommandContext(cmd.Context(), "bash", "-c", cmds).CombinedOutput()
		//if err != nil {
		//	pterm.Error.Printf(fmt.Sprintf("%s", out))
		//	pterm.Error.Println(err)
		//	return
		//}

		if strings.Contains(fmt.Sprintf("%s", out), "warning") {
			pterm.Warning.Print(fmt.Sprintf("Generating documentation from protos...\n%s", out))
		} else {
			pterm.Debug.Print(fmt.Sprintf("%s\n", out))
		}

		pterm.Success.Printf("Generated documentation at %s\n", homeDir+"/alis.exchange/"+organisationID+"/products/"+productID)

		return
	},
}

func init() {
	rootCmd.AddCommand(productCmd)
	productCmd.AddCommand(createProductCmd)
	productCmd.AddCommand(getProductCmd)
	productCmd.AddCommand(clearProductCmd)
	productCmd.AddCommand(listProductCmd)
	productCmd.AddCommand(treeProductCmd)
	productCmd.AddCommand(buildProductCmd)
	productCmd.AddCommand(deployProductCmd)
	productCmd.AddCommand(getkeyProductCmd)
	productCmd.AddCommand(gendocsProductCmd)
	productCmd.SilenceUsage = true
	productCmd.SilenceErrors = true

	buildProductCmd.Flags().StringVarP(&releaseType, "release", "r", "patch", pterm.Green("The update type, one of patch, minor & major"))
	deployProductCmd.Flags().BoolVarP(&setDeployProductEnvFlag, "env", "e", false, pterm.Green("Set or update the ENV variables for the relevant deployment"))
}

//// buildProduct builds a new version of the neuron in the development deployment/project.
//func buildProduct(ctx context.Context, productID string) error {
//
//	// set the parameters
//	p := Parameters{
//		Organisation: strings.Split(productID, ".")[0],
//		Product:      strings.Split(productID, ".")[1],
//	}
//
//	// retrieve the current deployed version from the resource in the development deployment.
//	deployment, err := alisDeploymentResourcesClient.GetDeployment(ctx, &pbProducts.GetDeploymentRequest{
//		Name: "organisations/" + p.Organisation + "/products/" + p.Product + "/deployments/" + p.Organisation + "-" + p.Product + "-dev",
//	})
//	if status.Code(err) == codes.NotFound {
//		// Create a Development deployment if one does not exist.
//		spinner, _ := pterm.DefaultSpinner.Start("Development deployment does not exist. Creating one now: %s ", "organisations/" + p.Organisation + "/products/" + p.Product + "/deployments/" + p.Organisation + "-" + p.Product + "-dev")
//		deployment, err = alisDeploymentResourcesClient.CreateDeployment(ctx, &pbProducts.CreateDeploymentRequest{
//			Parent: "organisations/" + p.Organisation + "/products/" + p.Product,
//			Deployment: &pbProducts.Deployment{
//				GoogleProjectId: p.Organisation + "-" + p.Product + "-dev",
//				Environment:     pbProducts.Deployment_DEVELOPMENT,
//				State:           pbProducts.Deployment_CREATING,
//				Owner:           "jan@alis.capital",
//				Version:         "1.0.0",
//			},
//			DeploymentId: p.Organisation + "-" + p.Product + "-dev",
//		})
//		if err != nil {
//			return err
//		}
//		spinner.Success("Created deployment resource: %s", "organisations/" + p.Organisation + "/products/" + p.Product + "/deployments/" + p.Organisation + "-" + p.Product + "-dev")
//	} else if err != nil {
//		return err
//	}
//
//	// Generate new version
//	newVersion, err := bumpVersion(deployment.GetVersion(), "patch")
//
//	// Update development neuron with new version
//	_, err = alisDeploymentResourcesClient.UpdateDeployment(ctx, &pbProducts.UpdateDeploymentRequest{
//		Deployment: &pbProducts.Deployment{
//			Name:    deployment.GetName(),
//			Version: newVersion,
//		},
//		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"version"}},
//	})
//	if err != nil {
//		return err
//	}
//
//	pterm.Info.Printf("v%s -> v%s... \n", deployment.GetVersion(), newVersion)
//
//	// commit and push a build spec.
//	productPath := fmt.Sprintf("%s/%s/%s", homeDir, p.Organisation, p.Product)
//	tag := fmt.Sprintf("%s.%s.%s", p.Organisation, p.Product, newVersion)
//
//	message := fmt.Sprintf("build(%s): %s", productID, newVersion)
//	err = commitTagAndPush(ctx, productPath, productPath, message, tag)
//	if err != nil {
//		return err
//	}
//
//
//	// commit and push a proto spec.
//	protoProductPath := fmt.Sprintf("%s/%s/proto/%s/%s", homeDir, p.Organisation, p.Organisation, p.Product)
//	protoPath := fmt.Sprintf("%s/%s/proto", homeDir, p.Organisation)
//	message = fmt.Sprintf("build(%s): %s", productID, newVersion)
//	err = commitTagAndPush(ctx, protoPath, protoProductPath, message, tag)
//	if err != nil {
//		return err
//	}
//
//	// Build the artifacts and deploy to dev environment
//	op, err := alisServicesClient.DeployProduct(ctx, &pbOSServices.DeployProductRequest{
//		Product: deployment.GetName(),
//		Version: newVersion,
//	})
//	if err != nil {
//		return err
//	}
//
//	if !asyncFlag || deployProductFlag {
//		// wait until operation is done.
//		spinner, _ := pterm.DefaultSpinner.Start("Building Product...")
//		for !op.GetDone() {
//			time.Sleep(10 * time.Second)
//			op, err = alisOperationsClient.GetOperation(ctx, &pbOperations.GetOperationRequest{Name: op.GetName()})
//			if err != nil {
//				spinner.Fail(err)
//				return err
//			}
//			if op.GetError() != nil {
//				spinner.Fail(op.GetError().GetMessage())
//				return fmt.Errorf("")
//			}
//		}
//		spinner.Success("Product built.")
//	}
//	return nil
//}
//
//// deployProduct deploys the staged product to all production environments
//func deployProduct(ctx context.Context, productID string) error {
//	// set the parameters
//	p := Parameters{
//		Organisation: strings.Split(productID, ".")[0],
//		Product:      strings.Split(productID, ".")[1],
//	}
//
//	// Retrieve the staged product
//	// TODO: once staging is setup, update this to a staging hit.
//	// retrieve the current deployed version from the product resource in the development deployment.
//	deployment, err := alisDeploymentResourcesClient.GetDeployment(ctx, &pbProducts.GetDeploymentRequest{
//		Name: "organisations/" + p.Organisation + "/products/" + p.Product + "/deployments/" + p.Organisation + "-" + p.Product + "-dev",
//	})
//	if err != nil {
//		return err
//	}
//
//	pterm.Info.Printf("Deploying v%s...\n", deployment.GetVersion())
//
//	// Retrieve a list of deployments
//	deployments, err := alisDeploymentResourcesClient.ListDeployments(ctx, &pbProducts.ListDeploymentsRequest{
//		Parent: "organisations/" + p.Organisation + "/products/" + p.Product,
//	})
//	if err != nil {
//		return err
//	}
//
//	// Deploy the product to all production environments
//	var wg sync.WaitGroup
//	progressBar := pterm.DefaultProgressbar.WithTotal(0).WithTitle("Deploying...")
//	for _, depl := range deployments.GetDeployments() {
//		if depl.Environment == pbProducts.Deployment_PRODUCTION {
//
//			// Deploy in parallel.
//			wg.Add(1)
//			progressBar.Total += 1
//
//			go func(dep *pbProducts.Deployment){
//				defer wg.Done()
//				// Update development neuron with new version
//				_, err = alisDeploymentResourcesClient.UpdateDeployment(ctx, &pbProducts.UpdateDeploymentRequest{
//					Deployment: &pbProducts.Deployment{
//						Name:    dep.GetName(),
//						Version: deployment.GetVersion(),
//						State: pbProducts.Deployment_CREATING,
//					},
//					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"version", "state"}},
//				})
//				if err != nil {
//					pterm.Error.Println(err)
//					return
//				}
//
//				// Deploy infrastructure
//				op, err := alisServicesClient.DeployProduct(ctx, &pbOSServices.DeployProductRequest{
//					Product: dep.GetName(),
//					Version: deployment.GetVersion(),
//				})
//				if err != nil {
//					pterm.Error.Println(err)
//					return
//				}
//				if !asyncFlag {
//					// wait until operation is done.
//					//spinnerSuccess, _ := pterm.DefaultSpinner.Start("Deploying " + dep.GetName() + "...")
//					for !op.GetDone() {
//						time.Sleep(10 * time.Second)
//						op, err = alisOperationsClient.GetOperation(ctx, &pbOperations.GetOperationRequest{Name: op.GetName()})
//						if err != nil {
//							//spinnerSuccess.Fail()
//							return
//						}
//					}
//					if op.GetError() != nil {
//						pterm.Error.Println(op.GetError().GetMessage())
//						return
//					}
//					pterm.Success.Println("Deployed " + dep.GetName())
//				}
//			}(depl)
//		}
//	}
//
//	_, _ = progressBar.Start()
//	wg.Wait()
//	return nil
//}
