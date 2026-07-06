package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var nodeGetCmd = &cobra.Command{
	Use:   "get [node name]",
	Short: "Get node configuration XML",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		nodeName := args[0]
		// Use script console to get raw XML because gojenkins doesn't expose it for computers
		script := fmt.Sprintf(`
import com.thoughtworks.xstream.XStream
def node = Jenkins.instance.getNode('%s')
if (node == null) { println "Node not found"; return }
println Jenkins.XSTREAM.toXML(node)
`, nodeName)

		output, err := client.ExecuteGroovy(script)
		if err != nil {
			return err
		}

		fmt.Println(output)
		return nil
	},
}

func init() {
	nodeCmd.AddCommand(nodeGetCmd)
}
