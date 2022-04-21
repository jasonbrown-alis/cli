package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	pbOperations "go.protobuf.alis.alis.exchange/alis/os/resources/operations/v1"
	pbProducts "go.protobuf.alis.alis.exchange/alis/os/resources/products/v1"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GoMod represents content in a go.mod file.
type GoMod struct {
	Module struct {
		Path string `json:"Path"`
	} `json:"Module"`
	Go      string `json:"Go"`
	Require []struct {
		Path     string `json:"Path"`
		Version  string `json:"Version"`
		Indirect bool   `json:"Indirect,omitempty"`
	} `json:"Require"`
	Exclude []struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	} `json:"Exclude"`
	Replace []struct {
		Old struct {
			Path string `json:"Path"`
		} `json:"Old"`
		New struct {
			Path string `json:"Path"`
		} `json:"New"`
	} `json:"Replace"`
	Retract []struct {
		Low  string `json:"Low"`
		High string `json:"High"`
	} `json:"Retract"`
}

// commitTagAndPush is a utility to manage commits, tagging and git push commands.
// Returns the commit hash.
func commitTagAndPush(ctx context.Context, repoPath string, commitPath string, message string, tag string, add bool, commit bool) (string, error) {

	// Pull the latest changes to local environment
	spinner, _ := pterm.DefaultSpinner.Start("Updating repositories updates for " + repoPath)
	cmds := "git -C " + repoPath + " pull --no-rebase"
	pterm.Debug.Printf("Shell command:\n%s\n", cmds)
	out, err := exec.CommandContext(ctx, "bash", "-c", cmds).CombinedOutput()
	if err != nil {
		pterm.Debug.Println(fmt.Sprintf("%s", out))
		return "", err
	}

	// Commit changes.
	if commit {
		spinner.UpdateText("Commit changes for " + commitPath)
		cmds = "git -C " + repoPath + " pull --no-rebase"
		if add {
			cmds = cmds + " && git -C " + repoPath + " add -- " + commitPath
		}
		cmds = cmds + " && git -C " + repoPath + " commit -m '" + message + "' -- " + commitPath
		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err := exec.CommandContext(ctx, "bash", "-c", cmds).CombinedOutput()
		if strings.Contains(fmt.Sprintf("%s", out), "Already up to date.") {
			pterm.Warning.Println(fmt.Sprintf("%s", out))
		} else if err != nil {
			pterm.Debug.Println(fmt.Sprintf("%s", out))
			return "", err
		}
	}

	// Push changes.
	spinner.UpdateText("Pushing changes for " + repoPath)
	if tag != "" {
		cmds = "git -C " + repoPath + " tag '" + tag + "' && " +
			"git -C " + repoPath + " push origin refs/heads/master:master --tags"
	} else {
		cmds = "git -C " + repoPath + " push origin refs/heads/master:master"
	}
	pterm.Debug.Printf("Shell command:\n%s\n", cmds)
	out, err = exec.CommandContext(ctx, "bash", "-c", cmds).CombinedOutput()
	if strings.Contains(fmt.Sprintf("%s", out), "already exists") {
		spinner.Warning(fmt.Sprintf("%s", out))
		return "", status.Errorf(codes.AlreadyExists, fmt.Sprintf("%s", out))
	}
	if err != nil {
		spinner.Fail(fmt.Sprintf("%s", out))
		return "", err
	}
	spinner.Success("Pushed repository " + pterm.LightGreen(repoPath) + " with tag " + pterm.LightGreen(tag))

	// Return the hash of the commit if a tag was provided.
	if tag != "" {
		cmds = "git -C " + repoPath + " rev-parse " + tag
		pterm.Debug.Printf("Shell command:\n%s\n", cmds)
		out, err = exec.CommandContext(ctx, "bash", "-c", cmds).Output()
		if err != nil {
			pterm.Debug.Println(fmt.Sprintf("%s", out))
		}
		pterm.Debug.Printf(string(out))

		sha := strings.Replace(string(out), "\n", "", -1)
		// sha should not be empty
		if sha == "" {
			return "", fmt.Errorf("the following command did not return a valid sha:\n%s\nplease run the following command to update the repo and try again:\n%s", cmds, "git -C "+repoPath+" pull --no-rebase")
		}

		return sha, nil
	} else {
		return "", nil
	}
}

// bumpVersion is a utility to increment the specified version by the releaseType
// inline with semantic versioning.
func bumpVersion(version string, releaseType string) (string, error) {
	major, err := strconv.Atoi(strings.Split(version, ".")[0])
	if err != nil {
		return "", err
	}

	minor, err := strconv.Atoi(strings.Split(version, ".")[1])
	if err != nil {
		return "", err
	}

	patch, err := strconv.Atoi(strings.Split(version, ".")[2])
	if err != nil {
		return "", err
	}

	// increment version inline with semantic versioning
	switch releaseType {
	case "minor":
		minor++
	case "patch":
		patch++
	default:
		return "", fmt.Errorf("release type %s not supported", releaseType)
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}

// waits for operation to complete
func wait(ctx context.Context, operation *longrunning.Operation, startMessage string, successMessage string, timeout int, useSpinner bool) error {
	// TODO: implement timeout
	_ = timeout
	if useSpinner {
		var err error
		spinner, _ := pterm.DefaultSpinner.Start(startMessage)
		for !operation.GetDone() {
			time.Sleep(5 * time.Second)
			operation, err = alisOperationsClient.GetOperation(ctx, &pbOperations.GetOperationRequest{Name: operation.GetName()})
			if err != nil {
				spinner.Fail(err.Error())
				return err
			}
			// TODO: improve user logging by updating text with metadata details of the operations object.
			if operation.GetError() != nil {
				spinner.Fail(operation.GetError().GetMessage())
				return fmt.Errorf(operation.GetError().GetMessage())
			}
		}
		spinner.Success(successMessage)
	} else {
		var err error
		for !operation.GetDone() {
			time.Sleep(5 * time.Second)
			operation, err = alisOperationsClient.GetOperation(ctx, &pbOperations.GetOperationRequest{Name: operation.GetName()})
			if err != nil {
				return err
			}
			// TODO: improve user logging by updating text with metadata details of the operations object.
			if operation.GetError() != nil {
				return fmt.Errorf(operation.GetError().GetMessage())
			}
		}
	}
	return nil
}

// validateArgument validates an argument and returns an error if not valid.
func validateArgument(value string, regex string) error {
	// validate the Name field using regex
	validateName := regexp.MustCompile(regex)
	validatedName := validateName.MatchString(value)
	if !validatedName {
		return status.Errorf(
			codes.InvalidArgument,
			"argument (%s) is not of the right format: %s", value, regex)
	}
	return nil
}

// selectProductDeployments retrieves a list of deployments for a particular product and
// ask the user to select one or more
// parent is the name of the Product resource
func selectProductDeployments(ctx context.Context, parent string) ([]*pbProducts.ProductDeployment, error) {
	// list the deployments and ask user to select one.
	productDeployments, err := alisProductsClient.ListProductDeployments(ctx, &pbProducts.ListProductDeploymentsRequest{
		Parent: parent,
	})

	if len(productDeployments.GetProductDeployments()) == 0 {
		pterm.Warning.Printf("the product (%s) has no deployments\n", parent)
		input, err := askUserString("Create a new ProductDeployment? (y|n):", `^y|n$`)
		if err != nil {
			return nil, err
		}

		if input == "y" {
			productDeployment, err := createProductDeployment(ctx, parent)
			if err != nil {
				return nil, err
			}
			return []*pbProducts.ProductDeployment{productDeployment}, nil
		} else {
			return nil, status.Errorf(codes.NotFound, "product % has no deployments", parent)
		}
	}

	table := pterm.TableData{{"Index", "Display Name", "Environment", "Deployment Project", "Owner", "Version", "State"}}
	for i, depl := range productDeployments.GetProductDeployments() {

		row := []string{strconv.Itoa(i), depl.GetDisplayName(), depl.GetEnvironment().String(), depl.GetGoogleProjectId(), depl.GetOwner(), depl.GetVersion(), depl.GetState().String()}

		if depl.GetState() != pbProducts.ProductDeployment_RUNNING {
			for i, col := range row {
				row[i] = pterm.Gray(col)
			}
		}
		table = append(table, row)
	}

	err = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
	if err != nil {
		return nil, err
	}

	input, err := askUserString("Please select one or more deployments (use comma seperated indices, for example 1,2,5)\n"+
		"or type 'NEW' to create a new deployment: ", `^NEW$|^(?:[0-9]+,)*[0-9]+$`)
	if err != nil {
		return nil, err
	}

	if input == "NEW" {
		res, err := createProductDeployment(ctx, parent)
		if err != nil {
			return nil, err
		}
		return []*pbProducts.ProductDeployment{res}, nil
	} else {

		var productDeploymentsSelection []*pbProducts.ProductDeployment

		for _, s := range strings.Split(input, ",") {
			i, err := strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
			if i >= len(productDeployments.GetProductDeployments()) {
				return nil, status.Errorf(codes.InvalidArgument, "%v is not a valid index selection", i)
			}
			productDeploymentsSelection = append(productDeploymentsSelection, productDeployments.GetProductDeployments()[i])
		}
		return productDeploymentsSelection, nil
	}
}

// askUserString ask the user for feedback and returns the response as a string.
func askUserString(question string, regex string) (string, error) {

	var input string
	for {
		var err error
		ptermInput.Printf(question)
		reader := bufio.NewReader(os.Stdin)
		input, err = reader.ReadString('\n')
		input = strings.Replace(input, "\n", "", -1)
		if err != nil {
			return "", err
		}
		valid := regexp.MustCompile(regex).MatchString(input)
		if !valid {
			pterm.Error.Println(status.Errorf(
				codes.InvalidArgument,
				"your response (%s) is not of the right format: %s\nPlease retry...", input, regex))
		} else {
			break
		}
	}

	return input, nil
}

// validateOrgArg is a utility used by the cobra command to validate Arguments.
func validateOrgArg(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		pterm.Error.Println("requires an organisation argument in the format: ^[a-z][a-z0-9]{2,7}$")
		return fmt.Errorf("")
	}

	err := validateArgument(args[0], "^[a-z][a-z0-9]{2,7}$")
	if err != nil {
		pterm.Error.Println(err)
		return fmt.Errorf("")
	}

	return nil
}

// validateProductArg is a utility used by the cobra command to validate Arguments.
func validateProductArg(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		pterm.Error.Println("requires an organisation.product argument in the format: ^[a-z][a-z0-9]{2,7}.[a-z]{2}$")
		return fmt.Errorf("")
	}

	err := validateArgument(args[0], `^[a-z][a-z0-9]{2,7}\.[a-z]{2}$`)
	if err != nil {
		pterm.Error.Println(err)
		return fmt.Errorf("")
	}

	return nil
}

// validateNeuronArg is a utility used by the cobra command to validate Arguments.
func validateNeuronArg(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		pterm.Error.Println("requires an organisation.product.neuron argument in the format: ^[a-z]+.[a-z]{2}.(resources|services)-[a-z]+-v[0-9]+$")
		return fmt.Errorf("")
	}

	err := validateArgument(args[0], `^[a-z][a-z0-9]{2,7}\.[a-z]{2}\.(resources|services)-[a-z]+-v[0-9]+$`)
	if err != nil {
		pterm.Error.Println(err)
		return fmt.Errorf("")
	}
	return nil
}

// validateOrgOrProductOrNeuron is a utility used by the cobra command to validate Arguments.
func validateOrgOrProductOrNeuron(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		pterm.Error.Println(`requires an organisation product or neuron argument in the format: ^([a-z][a-z0-9]{2,7})(\.[a-z]{2})*(\.(resources|services)-[a-z]+-v[0-9]+)*$`)
		return fmt.Errorf("")
	}

	err := validateArgument(args[0], `^([a-z][a-z0-9]{2,7})(\.[a-z]{2})*(\.(resources|services)-[a-z]+-v[0-9]+)*$`)
	if err != nil {
		pterm.Error.Println(err)
		return fmt.Errorf("")
	}

	return nil
}

// createProductDeployment creates a new product deployment and waits until done.
func createProductDeployment(ctx context.Context, productName string) (*pbProducts.ProductDeployment, error) {

	// retrieve a copy of the Product Resource
	product, err := alisProductsClient.GetProduct(ctx, &pbProducts.GetProductRequest{Name: productName})
	if err != nil {
		return nil, err
	}

	// Get additional user input
	pterm.Info.Println("Great. Let's create a new deployment.  Please provide the following for the deployment:")

	env := pbProducts.ProductDeployment_DEV
	envStr, err := askUserString("Development or Production environment? (PROD|DEV): ", `^PROD$|^DEV$`)
	if err != nil {
		return nil, err
	}
	if envStr == "PROD" {
		env = pbProducts.ProductDeployment_PROD
	}

	displayName, err := askUserString("Display Name: ", `^[A-Za-z0-9- ]+$`)
	if err != nil {
		return nil, err
	}
	owner, err := askUserString("Owner (email): ", `(?m)^([a-zA-Z0-9_\-\.]+)@([a-zA-Z0-9_\-\.]+)\.([a-zA-Z]{2,10})$`)
	if err != nil {
		return nil, err
	}
	ptermTip.Printf("The Product (%s) has a billing account ID of %s\n", product.GetName(), strings.Split(product.GetBillingAccount(), "/")[1]+"\nNavigate to https://console.cloud.google.com/billing to see the billing accounts available to you.")
	billingAccountID, err := askUserString("ProductDeployment Billing Account ID: ", `^[A-Z0-9]{6}-[A-Z0-9]{6}-[A-Z0-9]{6}$`)
	if err != nil {
		return nil, err
	}

	op, err := alisProductsClient.CreateProductDeployment(ctx, &pbProducts.CreateProductDeploymentRequest{
		Parent: product.GetName(),
		ProductDeployment: &pbProducts.ProductDeployment{
			Environment:    env,
			Owner:          owner,
			DisplayName:    displayName,
			BillingAccount: "billingAccounts/" + billingAccountID,
		},
	})
	if err != nil {
		return nil, err
	}

	// wait for the long-running operation to complete.
	err = wait(ctx, op, "Creating a new Product Deployment", "Created a new Product Deployment", 300, true)
	if err != nil {
		return nil, err
	}

	res, err := alisOperationsClient.GetOperation(ctx, &pbOperations.GetOperationRequest{Name: op.GetName()})
	if err != nil {
		return nil, err
	}

	productDeployment := &pbProducts.ProductDeployment{}
	err = res.GetResponse().UnmarshalTo(productDeployment)
	if err != nil {
		return nil, err
	}

	return productDeployment, nil
}

// createInitialProductDeployment creates a new development product deployment for a new product
func createInitialProductDeployment(ctx context.Context, productName string) (*pbProducts.ProductDeployment, error) {

	// retrieve a copy of the Product Resource
	product, err := alisProductsClient.GetProduct(ctx, &pbProducts.GetProductRequest{Name: productName})
	if err != nil {
		return nil, err
	}

	// Get additional user input
	pterm.Info.Println("Great. Let's create a new deployment.  Please provide the following for the deployment:")

	env := pbProducts.ProductDeployment_DEV
	envStr, err := askUserString("Development or Production environment? (PROD|DEV): ", `^PROD$|^DEV$`)
	if err != nil {
		return nil, err
	}
	if envStr == "PROD" {
		env = pbProducts.ProductDeployment_PROD
	}

	displayName, err := askUserString("Display Name: ", `^[A-Za-z0-9- ]+$`)
	if err != nil {
		return nil, err
	}
	owner, err := askUserString("Owner (email): ", `(?m)^([a-zA-Z0-9_\-\.]+)@([a-zA-Z0-9_\-\.]+)\.([a-zA-Z]{2,10})$`)
	if err != nil {
		return nil, err
	}
	ptermTip.Printf("The Product (%s) has a billing account ID of %s\n", product.GetName(), strings.Split(product.GetBillingAccount(), "/")[1]+"\nNavigate to https://console.cloud.google.com/billing to see the billing accounts available to you.")
	billingAccountID, err := askUserString("ProductDeployment Billing Account ID: ", `^[A-Z0-9]{6}-[A-Z0-9]{6}-[A-Z0-9]{6}$`)
	if err != nil {
		return nil, err
	}

	op, err := alisProductsClient.CreateProductDeployment(ctx, &pbProducts.CreateProductDeploymentRequest{
		Parent: product.GetName(),
		ProductDeployment: &pbProducts.ProductDeployment{
			Environment:    env,
			Owner:          owner,
			DisplayName:    displayName,
			BillingAccount: "billingAccounts/" + billingAccountID,
		},
	})
	if err != nil {
		return nil, err
	}

	// wait for the long-running operation to complete.
	err = wait(ctx, op, "Creating a new Product Deployment", "Created a new Product Deployment", 300, true)
	if err != nil {
		return nil, err
	}

	res, err := alisOperationsClient.GetOperation(ctx, &pbOperations.GetOperationRequest{Name: op.GetName()})
	if err != nil {
		return nil, err
	}

	productDeployment := &pbProducts.ProductDeployment{}
	err = res.GetResponse().UnmarshalTo(productDeployment)
	if err != nil {
		return nil, err
	}

	return productDeployment, nil
}

// askUserProductEnvs list the current envs and ask for updated values.
func askUserProductEnvs(envs []*pbProducts.Product_Env) ([]*pbProducts.Product_Env, error) {

	var res []*pbProducts.Product_Env

	if len(envs) > 0 {
		table := pterm.TableData{{"Index", "Environment Variable", "Current Value"}}
		for i, env := range envs {
			table = append(table, []string{strconv.Itoa(i), env.GetName(), env.GetValue()})
		}
		err := pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
		if err != nil {
			return nil, err
		}

		pterm.Info.Println("Please provide new values for the above variables:\n- leave blank (enter) to keep unchanged\n- 'REMOVE' to remove")

		for _, env := range envs {
			input, err := askUserString(env.GetName()+": ", `^.*$`)
			if err != nil {
				return nil, err
			}
			if input == "REMOVE" {
				continue
			}

			if input != "" {
				res = append(res, &pbProducts.Product_Env{
					Name:  env.GetName(),
					Value: input,
				})
			} else {
				res = append(res, env)
			}
		}
	} else {
		pterm.Warning.Printf("there are no environmental variables set\n")
	}

	// ask for new values?

	for {
		input, err := askUserString("Add a new environmental variable? (y|n): ", "^[y|n]$")
		if err != nil {
			return nil, err
		}
		if input == "y" {
			name, err := askUserString("Name (starting with 'ALIS_OS_'): ", "^ALIS_OS_[A-Z0-9_]+$")
			if err != nil {
				return nil, err
			}
			value, err := askUserString("Value: ", `^$|^[a-zA-Z0-9:._\/-]+$`)
			if err != nil {
				return nil, err
			}
			res = append(res, &pbProducts.Product_Env{
				Name:  name,
				Value: value,
			})
			ptermTip.Printf("Using Google Cloud Run?\nPlease remember to add %s to the Cloud Run Terraform resource\n", name)
		} else {
			// break out of the for loop
			break
		}
	}

	pterm.Debug.Printf("Updated values:\n%s\n", res)

	return res, nil
}

// askUserNeuronEnvs list the current envs and ask for updated values.
func askUserNeuronEnvs(envs []*pbProducts.Neuron_Env) ([]*pbProducts.Neuron_Env, error) {

	var res []*pbProducts.Neuron_Env

	if len(envs) > 0 {
		table := pterm.TableData{{"Index", "Environment Variable", "Current Value"}}
		for i, env := range envs {
			table = append(table, []string{strconv.Itoa(i), env.GetName(), env.GetValue()})
		}
		err := pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
		if err != nil {
			return nil, err
		}

		pterm.Info.Println("Please provide new values for the above variables:\n- leave blank (enter) to keep unchanged\n- 'REMOVE' to remove")

		for _, env := range envs {
			input, err := askUserString(env.GetName()+": ", `^.*$`)
			if err != nil {
				return nil, err
			}
			if input == "REMOVE" {
				continue
			}

			if input != "" {
				res = append(res, &pbProducts.Neuron_Env{
					Name:  env.GetName(),
					Value: input,
				})
			} else {
				res = append(res, env)
			}
		}
	} else {
		pterm.Warning.Printf("there are no environmental variables set\n")
	}

	// ask for new values?

	for {
		input, err := askUserString("Add a new environmental variable? (y|n): ", "^[y|n]$")
		if err != nil {
			return nil, err
		}
		if input == "y" {
			name, err := askUserString("Name (starting with 'ALIS_OS_'): ", "^ALIS_OS_[A-Z0-9_]+$")
			if err != nil {
				return nil, err
			}
			value, err := askUserString("Value: ", `^$|^[a-zA-Z0-9:._\/-]+$`)
			if err != nil {
				return nil, err
			}
			res = append(res, &pbProducts.Neuron_Env{
				Name:  name,
				Value: value,
			})
			ptermTip.Printf("Using Google Cloud Run?\nPlease remember to add %s to the Cloud Run Terraform resource\n", name)
		} else {
			// break out of the for loop
			break
		}
	}

	pterm.Debug.Printf("Updated values:\n%s\n", res)

	return res, nil
}

// askUserNeuronState list the current envs and ask for updated values.
func askUserNeuronState(state pbProducts.Neuron_State) (pbProducts.Neuron_State, error) {

	table := pterm.TableData{{"Index", "Neuron States", "Current State"}}
	for i := 0; i < len(pbProducts.Neuron_State_name); i++ {
		currentState := ""
		if state == pbProducts.Neuron_State(i) {
			currentState = pterm.LightGreen("\u2713")
		}
		table = append(table, []string{
			fmt.Sprintf("%v", i),
			fmt.Sprintf("%v", pbProducts.Neuron_State_name[int32(i)]),
			currentState,
		})
	}

	err := pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
	if err != nil {
		return 0, err
	}

	var selection int
	for {
		input, err := askUserString("Please select a state (use Index): ", "^[0-9]+$")
		if err != nil {
			return 0, err
		}
		selection, err = strconv.Atoi(input)

		if selection >= len(pbProducts.Neuron_State_name) {
			pterm.Error.Printf("%v is an invalid selection, please try again...\n", selection)
		} else {
			break
		}
	}

	var res pbProducts.Neuron_State
	res = pbProducts.Neuron_State(selection)

	pterm.Debug.Printf("User selected values:\n%s\n", res.String())
	return res, nil
}

// askUserNeuronDeploymentState list the current envs and ask for updated values.
func askUserNeuronDeploymentState(state pbProducts.NeuronDeployment_State) (pbProducts.NeuronDeployment_State, error) {

	table := pterm.TableData{{"Index", "Neuron Deployment States", "Current State"}}
	for i := 0; i < len(pbProducts.NeuronDeployment_State_name); i++ {
		currentState := ""
		if state == pbProducts.NeuronDeployment_State(i) {
			currentState = pterm.LightGreen("\u2713")
		}
		table = append(table, []string{
			fmt.Sprintf("%v", i),
			fmt.Sprintf("%v", pbProducts.NeuronDeployment_State_name[int32(i)]),
			currentState,
		})
	}

	err := pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(table).Render()
	if err != nil {
		return 0, err
	}

	var selection int
	for {
		input, err := askUserString("Please select a state (use Index): ", "^[0-9]+$")
		if err != nil {
			return 0, err
		}
		selection, err = strconv.Atoi(input)

		if selection >= len(pbProducts.NeuronDeployment_State_name) {
			pterm.Error.Printf("%v is an invalid selection, please try again...\n", selection)
		} else {
			break
		}
	}

	var res pbProducts.NeuronDeployment_State
	res = pbProducts.NeuronDeployment_State(selection)

	pterm.Debug.Printf("User selected values:\n%s\n", res.String())
	return res, nil
}

// findNeuronDockerFilePaths searches recursively within a Neuron for any 'Dockerfile' files.
func findNeuronDockerFilePaths(neuron string) ([]string, error) {
	orgId := strings.Split(neuron, ".")[0]
	productId := strings.Split(neuron, ".")[1]
	neuronId := strings.Join(strings.Split(neuron, ".")[2:], "/")

	root := fmt.Sprintf("%s/alis.exchange/%s/products/%s/%s", homeDir, orgId, productId, neuronId)

	var paths []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "Dockerfile" {
			// remove the root and Dockerfile components from the path.
			p := strings.Replace(path, root, ".", 1)
			p = strings.Replace(p, "/Dockerfile", "", 1)
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// getNeuronDescriptor creates a temp descriptor.pb file to parse the contents to a FileDescriptorSet object.
func getNeuronDescriptor(neuron string) (*descriptorpb.FileDescriptorSet, error) {

	organisationID := strings.Split(neuron, "/")[1]
	productID := strings.Split(neuron, "/")[3]
	neuronID := strings.Split(neuron, "/")[5]
	//neuronProtoFullPath := homeDir + "/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" +
	//	productID + "/" + strings.ReplaceAll(neuronID, "-", "/") + "/descriptor.pb"

	// Generate the descriptor.pb at neuron level
	// This descriptor file represents the .proto files at the point in time
	// which will be used when creating a new NeuronVersion resource.
	neuronProtoFullPath := homeDir + "/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/")
	cmds := "protoc --descriptor_set_out=$HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/" + strings.ReplaceAll(neuronID, "-", "/") + "/descriptor.pb -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto --include_source_info " + neuronProtoFullPath + "/*.proto"
	pterm.Debug.Printf("Shell command:\n%s\n", cmds)
	out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
	if err != nil {
		if strings.Contains(fmt.Sprintf("%s", out), "No such file or directory") {
			pterm.Warning.Print(fmt.Sprintf("%s\n", out))
			return nil, nil
		} else {
			return nil, err
		}
	}

	b, err := ioutil.ReadFile(neuronProtoFullPath + "/descriptor.pb")
	if err != nil {
		return nil, err
	}

	res := &descriptorpb.FileDescriptorSet{}
	err = proto.Unmarshal(b, res)
	if err != nil {
		return nil, err
	}

	// delete the descriptor.pb file
	err = os.Remove(neuronProtoFullPath + "/descriptor.pb")
	if err != nil {
		return nil, err
	}

	return res, nil
}

// generateRandomId generates a random id of the specified length.
func generateRandomId(n int) string {
	rand.Seed(time.Now().UnixNano())
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// genProductDescriptorFile generates a descriptor.pb file at the product level.
func genProductDescriptorFile(product string) error {
	organisationID := strings.Split(product, "/")[1]
	productID := strings.Split(product, "/")[3]

	// Generate the descriptor.pb at product level
	// The descriptor.pb at product level represents all the underlying neurons.
	cmds := "go env -w GOPRIVATE=go.lib." + organisationID + ".alis.exchange,go.protobuf." + organisationID + ".alis.exchange,proto." + organisationID + ".alis.exchange,cli.alis.dev && " +
		"protoc --descriptor_set_out=$HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + "/descriptor.pb -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto --include_imports --include_source_info $(find $HOME/alis.exchange/" + organisationID + "/proto/" + organisationID + "/" + productID + " -iname \"*.proto\")"
	pterm.Debug.Printf("Shell command:\n%s\n", cmds)
	out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error %s, with command line output:\n %s", err, out)
	}
	return nil
}

// genDescriptorFile generates a descriptor.pb file at the neuron level.
func genDescriptorFile(name string) (string, error) {
	// parse the resource name
	nameParts := strings.Split(name, "/")
	organisationID = nameParts[1]
	protoPath := nameParts[1]
	// update the path if Product is provided
	if len(nameParts) >= 3 {
		protoPath += "/" + nameParts[3]
	}
	// update the path if Neuron is provided
	if len(nameParts) >= 5 {
		// Neuron name resources-events-v1 -> resources/events/v1
		protoPath += "/" + strings.ReplaceAll(nameParts[5], "-", "/")
	}

	// Generate the descriptor.pb at the relevant org/product/neuron level
	// The descriptor.pb at product level represents all the underlying neurons.
	cmds := "protoc --descriptor_set_out=$HOME/alis.exchange/" + organisationID + "/proto/" + protoPath + "/descriptor.pb -I=$HOME/alis.exchange/google/proto -I=$HOME/alis.exchange/" + organisationID + "/proto --include_imports --include_source_info $(find $HOME/alis.exchange/" + organisationID + "/proto/" + protoPath + " -iname \"*.proto\")"
	pterm.Debug.Printf("Shell command:\n%s\n", cmds)
	out, err := exec.CommandContext(context.Background(), "bash", "-c", cmds).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error %s, with command line output:\n %s", err, out)
	}

	return homeDir + "/alis.exchange/" + organisationID + "/proto/" + protoPath + "/descriptor.pb", nil
}

// validGitDirectory check that the provided directory is a valid git directory.
func validGitDirectory(dir string) (bool, error) {

	return true, nil
}

// getGoMod returns the contents of the go.mod file
func getGoMod(ctx context.Context, neuronPath string) (*GoMod, error) {

	goMod := &GoMod{}

	cmds := "go mod edit -json " + neuronPath + "/go.mod"
	out, err := exec.CommandContext(ctx, "bash", "-c", cmds).CombinedOutput()
	if err != nil {
		return nil, err
	}
	// marshall the commandline output to a GoMod type.
	err = json.Unmarshal(out, &goMod)
	if err != nil {
		return nil, err
	}
	return goMod, nil
}
