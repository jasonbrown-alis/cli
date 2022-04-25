package cmd

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	pbProducts "go.protobuf.alis.alis.exchange/alis/os/resources/products/v1"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

// neuronCmd represents the neuron command
var genCmd = &cobra.Command{
	Use:   "gen",
	Short: pterm.Blue("Generates code"),
	Long:  pterm.Green(`Use this command to generate code.`),
	Run: func(cmd *cobra.Command, args []string) {
		pterm.Error.Println("a valid command is missing\nplease run 'alis gen -h' for details.")
	},
}

// protobufGenCmd represents the genproto command
var protobufGenCmd = &cobra.Command{
	Use:   "protobuf",
	Short: pterm.Blue("Generates the protocol buffers files for a specified language.  Golang is the default."),
	Long: pterm.Green(
		`This method uses the 'genproto-go' command line to generate protocol buffers 
for the specified neuron.  These are generated locally only and should be used for local development of your gRPC services.

These are used in combination with the 'Replace ...' command in your go.mod file to 'point' to the local, instead of the
official protobufs.'

Once local development is done, use the '--push' flag to push the generated protocol buffers to the go.protobuf repository.
Once the protobufs are pushed, you should remove the 'Replace... ' command in your go.mod file and run 'go mod tidy' to pull
the latest protobufs from the repo into your gRPC service.`),
	Example: pterm.LightYellow("alis gen protobuf {orgID}.{productID}.{neuronID}"),
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

			// Clear all files in the relevant neuron folder.
			// TODO: refactor the use of GOPRIVATE envs.
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
			pterm.Success.Printf("Generated protocol buffers for Python.\nProto source: %s\n", neuronProtoFullPath)

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

// descriptorGenCmd represents the descriptor command
var descriptorGenCmd = &cobra.Command{
	Use:   "descriptor",
	Short: pterm.Blue("Generates a file descriptor for the specified organisation, product or neuron."),
	Long: pterm.Green(
		`This method uses the 'protoc --descriptor_set_out=...' command line to generate a local 'descriptor.pb' file.
This file is a serialised google.protobuf.FileDescriptorSet object representing all the relevant proto files.`),
	Example: pterm.LightYellow("alis gen descriptor {orgID}.{productID}.{neuronID}"),
	Args:    validateOrgOrProductOrNeuron,
	Run: func(cmd *cobra.Command, args []string) {

		var name string
		argParts := strings.Split(args[0], ".")

		// The length of the argument parts determine whether the request is at organisation, product or neuron level.
		switch len(argParts) {
		case 1:
			// Retrieve the organisation resource
			name = "organisations/" + argParts[0]
			organisation, err := alisProductsClient.GetOrganisation(cmd.Context(),
				&pbProducts.GetOrganisationRequest{Name: name})
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			pterm.Debug.Printf("Get Organisation:\n%s\n", organisation)
		case 2:
			// Retrieve the product resource
			name = "organisations/" + argParts[0] + "/products/" + argParts[1]
			product, err := alisProductsClient.GetProduct(cmd.Context(), &pbProducts.GetProductRequest{Name: name})
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			pterm.Debug.Printf("Get Product:\n%s\n", product)
		case 3:
			// Retrieve the neuron resource
			name = "organisations/" + argParts[0] + "/products/" + argParts[1] + "/neurons/" + argParts[2]
			neuron, err := alisProductsClient.GetNeuron(cmd.Context(),
				&pbProducts.GetNeuronRequest{
					Name: name})
			if err != nil {
				pterm.Error.Println(err)
				return
			}
			pterm.Debug.Printf("Get Neuron:\n%s\n", neuron)
		}

		// generate ProductDescriptorFile at the relevant resource level.
		descriptorPath, err := genDescriptorFile(name)
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		pterm.Info.Printf("Generated %s\n", descriptorPath)

		return
	},
}

func init() {
	rootCmd.AddCommand(genCmd)
	genCmd.AddCommand(protobufGenCmd)
	genCmd.AddCommand(descriptorGenCmd)
	neuronCmd.SilenceUsage = true
	neuronCmd.SilenceErrors = true

	protobufGenCmd.Flags().BoolVarP(&pushProtocolBuffers, "publish", "p", false, pterm.Green("Generate the protocol buffers and push them to the protobuf repository"))

	// protobuf flags
	protobufGenCmd.Flags().BoolVarP(&genprotoGo, "go", "", true, pterm.Green("Generate the protocol buffers for Golang"))
	protobufGenCmd.Flags().BoolVarP(&genprotoPython, "python", "", false, pterm.Green("Generate the protocol buffers for Python"))
	protobufGenCmd.Flags().BoolVarP(&genprotoCpp, "cpp", "", false, pterm.Green("Generate the protocol buffers for C++"))
}
