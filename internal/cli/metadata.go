package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/gitlog"
)

// metadataHelp explains the metadata of a run, for the help of the commands which start runs.
const metadataHelp = "Metadata is extra information shown with the run, for example the commits it deploys.\n" +
	"Add your own with --meta and --meta-file.\n\n" +
	"--meta-git collects it from the git repository you are in: the commit which is deployed\n" +
	"and the new commits since the last successful run. It reads the remote branch of your\n" +
	"current branch, or the ref you give, and does not change your checkout. The new commits\n" +
	"are only listed if the last run used --meta-git too. If there is a variable " + commitVariable + ", it is\n" +
	"set to the deployed commit."

// metadataFlags are the flags which give a run its metadata.
type metadataFlags struct {
	values []string
	files  []string
	git    string
}

// addMetadataFlags adds the flags which give a run its metadata.
func addMetadataFlags(cmd *cobra.Command, flags *metadataFlags) {
	cmd.Flags().StringArrayVar(&flags.values, "meta", nil, "metadata to show with the run, as name=value (repeatable)")
	cmd.Flags().StringArrayVar(&flags.files, "meta-file", nil, "metadata read from a file, as name=path (repeatable)")
	cmd.Flags().StringVar(&flags.git, "meta-git", "", "add the deployed commit and the new commits since the last run as metadata;\n"+
		"uses the remote branch of your current branch, or the ref you give")
	cmd.Flags().Lookup("meta-git").NoOptDefVal = gitlog.Upstream
}

// collectMetadata combines the collected metadata with the --meta-file and --meta flags, which
// win in this order over what was collected.
func collectMetadata(flags metadataFlags, collected map[string]string) (map[string]string, error) {
	metadata := map[string]string{}
	for name, value := range collected {
		metadata[name] = value
	}

	for _, flag := range flags.files {
		name, path, ok := strings.Cut(flag, "=")
		if !ok || name == "" || path == "" {
			return nil, usageError("Invalid metadata file “"+flag+"”", "Use --meta-file name=path.")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, usageError("The metadata file “"+path+"” can't be read", err.Error())
		}
		metadata[name] = strings.TrimRight(string(content), "\n")
	}
	for _, flag := range flags.values {
		name, value, ok := strings.Cut(flag, "=")
		if !ok || name == "" {
			return nil, usageError("Invalid metadata “"+flag+"”", "Use --meta name=value.")
		}
		metadata[name] = value
	}
	return metadata, nil
}
