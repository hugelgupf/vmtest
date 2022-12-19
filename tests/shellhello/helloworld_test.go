package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	//"github.com/u-root/u-root/pkg/uroot"
	//"github.com/u-root/u-root/pkg/uroot/builder"
)

func TestStartVM(t *testing.T) {
	vmtest.TestCmdsInVM(t, []string{`echo "Hello World"`}, nil)
	/*&vmtest.Options{
		BuildOpts: uroot.Opts{
			Commands: []uroot.Commands{
				uroot.Commands{
					Builder:  builder.BusyBox,
					Packages: []string{"github.com/u-root/u-root/cmds/core/echo"},
				},
			},
		},
	})*/
}
