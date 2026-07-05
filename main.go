package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AmadlaOrg/raise-quickemu/quickemu"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	appName = "raise-quickemu"
	version = "1.0.0"
)

var rootCmd = &cobra.Command{
	Use:     appName,
	Short:   "Raise plugin for managing VMs via quickemu/quickget",
	Version: version,
}

var (
	infoOutputFlag string
	infoHeryFlag   bool

	infoCmd = &cobra.Command{
		Use:   "info",
		Short: "Show plugin metadata",
		Run: func(cmd *cobra.Command, args []string) {
			metadata := map[string]any{
				"name":    appName,
				"version": version,
				"engine":  "quickemu",
				"supports": []string{
					"amadla.org/entity/infrastructure@^v1.0.0",
					"amadla.org/entity/infrastructure/vm@^v1.0.0",
				},
				"description": "Manages VMs via quickemu/quickget (upstream OS images)",
			}
			if err := writeInfoOutput(os.Stdout, infoOutputFlag, infoHeryFlag, metadata); err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding metadata: %v\n", err)
				os.Exit(1)
			}
		},
	}
)

var (
	upFilePath string

	upCmd = &cobra.Command{
		Use:   "up [name]",
		Short: "Create and start a VM",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runUp,
	}
)

var haltCmd = &cobra.Command{
	Use:   "halt <name>",
	Short: "Stop a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := quickemu.New()
		if err := mgr.Halt(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "VM %q is stopping\n", args[0])
		return nil
	},
}

var destroyCmd = &cobra.Command{
	Use:   "destroy <name>",
	Short: "Destroy a VM and remove all files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := quickemu.New()
		if err := mgr.Destroy(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "VM %q has been destroyed\n", args[0])
		return nil
	},
}

var (
	sshUser    string
	sshPort    int
	sshKeyFile string

	sshCmd = &cobra.Command{
		Use:   "ssh <name>",
		Short: "SSH into a VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := quickemu.New()
			if err := mgr.SSH(args[0], sshUser, sshPort, sshKeyFile); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}
)

var statusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show VM status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := quickemu.New()

		if len(args) == 1 {
			state, err := mgr.Status(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return quickemu.OutputJSON(state)
		}

		states, err := mgr.StatusAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return quickemu.OutputJSON(states)
	},
}

func init() {
	upCmd.Flags().StringVarP(&upFilePath, "file", "f", "", "Input data file (JSON or YAML; use '-' for stdin)")
	_ = upCmd.MarkFlagRequired("file")

	sshCmd.Flags().StringVarP(&sshUser, "user", "u", "", "SSH user (default: from entity or user)")
	sshCmd.Flags().IntVarP(&sshPort, "port", "p", 0, "SSH port (default: from quickemu, typically 22220)")
	sshCmd.Flags().StringVarP(&sshKeyFile, "key", "k", "", "SSH private key file")

	infoCmd.Flags().StringVarP(&infoOutputFlag, "output", "o", "table", "Output format: table, json, yaml")
	infoCmd.Flags().BoolVar(&infoHeryFlag, "hery", false, "Wrap output in HERY envelope (_type, _body)")

	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(haltCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(statusCmd)
}

func writeInfoOutput(w io.Writer, format string, hery bool, data map[string]any) error {
	if hery {
		return writeHeryOutput(w, format, data)
	}

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		bytes, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(w, string(bytes))
		return err
	default:
		table := tablewriter.NewWriter(w)
		table.Header("Field", "Value")
		table.Append("Name", fmt.Sprint(data["name"]))
		table.Append("Version", fmt.Sprint(data["version"]))
		table.Append("Engine", fmt.Sprint(data["engine"]))
		table.Append("Description", fmt.Sprint(data["description"]))
		if supports, ok := data["supports"].([]string); ok {
			table.Append("Supports", strings.Join(supports, "\n"))
		}
		table.Render()
		return nil
	}
}

type heryEnvelope struct {
	Type string `json:"_type" yaml:"_type"`
	Body any    `json:"_body" yaml:"_body"`
}

func writeHeryOutput(w io.Writer, format string, data map[string]any) error {
	envelope := heryEnvelope{
		Type: "amadla.org/entity/tools/info@v1.0.0",
		Body: data,
	}

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(envelope)
	case "table":
		fmt.Fprintf(w, "_type: %s\n\n", envelope.Type)
		table := tablewriter.NewWriter(w)
		table.Header("Field", "Value")
		table.Append("Name", fmt.Sprint(data["name"]))
		table.Append("Version", fmt.Sprint(data["version"]))
		table.Append("Engine", fmt.Sprint(data["engine"]))
		table.Append("Description", fmt.Sprint(data["description"]))
		if supports, ok := data["supports"].([]string); ok {
			table.Append("Supports", strings.Join(supports, "\n"))
		}
		table.Render()
		return nil
	default:
		bytes, err := yaml.Marshal(envelope)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(w, string(bytes))
		return err
	}
}

// entityInput represents the infrastructure entity format.
type entityInput struct {
	Type    string      `json:"_type" yaml:"_type"`
	Extends string      `json:"_extends" yaml:"_extends"`
	Meta    any         `json:"_meta" yaml:"_meta"`
	Body    *entityBody `json:"_body" yaml:"_body"`
}

type entityBody struct {
	OS       string     `json:"os" yaml:"os"`
	Release  string     `json:"release" yaml:"release"`
	Edition  string     `json:"edition" yaml:"edition"`
	CPUs     int        `json:"cpus" yaml:"cpus"`
	Memory   string     `json:"memory" yaml:"memory"`
	DiskSize string     `json:"disk_size" yaml:"disk_size"`
	Display  string     `json:"display" yaml:"display"`
	SSHPort  int        `json:"ssh_port" yaml:"ssh_port"`
	SSH      *entitySSH `json:"ssh" yaml:"ssh"`
}

type entitySSH struct {
	User       string `json:"user" yaml:"user"`
	Port       int    `json:"port" yaml:"port"`
	PrivateKey string `json:"private_key" yaml:"private_key"`
}

func runUp(cmd *cobra.Command, args []string) error {
	var input io.Reader

	if upFilePath == "-" {
		input = os.Stdin
	} else {
		f, err := os.Open(upFilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot open file: %v\n", err)
			os.Exit(2)
		}
		defer f.Close()
		input = f
	}

	data, err := io.ReadAll(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot read input: %v\n", err)
		os.Exit(2)
	}

	config, err := parseEntityToConfig(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot parse input: %v\n", err)
		os.Exit(2)
	}

	// Override name from positional argument if provided
	if len(args) == 1 {
		config.Name = args[0]
	}

	mgr := quickemu.New()
	state, err := mgr.Up(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	return quickemu.OutputJSON(state)
}

func parseEntityToConfig(data []byte) (*quickemu.VMConfig, error) {
	var entity entityInput

	if err := json.Unmarshal(data, &entity); err != nil {
		if err := yaml.Unmarshal(data, &entity); err != nil {
			return nil, fmt.Errorf("input is neither valid JSON nor YAML: %w", err)
		}
	}

	if entity.Body == nil {
		return nil, fmt.Errorf("input missing _body field")
	}

	body := entity.Body
	config := &quickemu.VMConfig{
		OS:       body.OS,
		Release:  body.Release,
		Edition:  body.Edition,
		CPUs:     body.CPUs,
		Memory:   body.Memory,
		DiskSize: body.DiskSize,
		Display:  body.Display,
		SSHPort:  body.SSHPort,
	}

	if body.SSH != nil {
		config.SSHUser = body.SSH.User
		config.SSHPort = body.SSH.Port
		config.SSHPrivateKey = body.SSH.PrivateKey
	}

	return config, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2)
	}
}
