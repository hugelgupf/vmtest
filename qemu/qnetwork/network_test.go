package qnetwork

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/uqemu"
	"github.com/u-root/u-root/pkg/uroot"
)

func TestInterVM(t *testing.T) {
	serverScript := `
set -x
ip addr add 192.168.0.1/24 dev eth0
ip link set eth0 up
pxeserver -4=false -http-dir=/etc
shutdown
`
	clientScript := `
set -x
ip addr add 192.168.0.2/24 dev eth0
ip link set eth0 up
ip a
wget http://192.168.0.1/hello
cat ./hello
ls -l /sys/class/net/eth0/device/driver
shutdown
`
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "server.sh"), []byte(serverScript), 0o777)
	_ = os.WriteFile(filepath.Join(d, "client.sh"), []byte(clientScript), 0o777)
	_ = os.WriteFile(filepath.Join(d, "hello"), []byte("all hello all world\n"), 0o777)

	for _, tt := range []struct {
		nic        NIC
		clientMAC  net.HardwareAddr
		serverMAC  net.HardwareAddr
		wantDriver string
	}{
		{
			nic:        NICE1000,
			wantDriver: "e1000",
		},
		{
			nic:        NICE1000,
			serverMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 0},
			clientMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 1},
			wantDriver: "e1000",
		},
		{
			nic:        NICVirtioNet,
			serverMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 0},
			clientMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 1},
			wantDriver: "virtio_net",
		},
	} {
		t.Run(fmt.Sprintf("%s-%s", tt.nic, tt.serverMAC), func(t *testing.T) {
			net := NewInterVM()
			initramfs := uroot.Opts{
				InitCmd:   "init",
				UinitCmd:  "gosh",
				UinitArgs: []string{"script.sh"},
				TempDir:   t.TempDir(),
				Commands: uroot.BusyBoxCmds(
					"github.com/u-root/u-root/cmds/core/cat",
					"github.com/u-root/u-root/cmds/core/gosh",
					"github.com/u-root/u-root/cmds/core/init",
					"github.com/u-root/u-root/cmds/core/ip",
					"github.com/u-root/u-root/cmds/core/ls",
					"github.com/u-root/u-root/cmds/core/shutdown",
					"github.com/u-root/u-root/cmds/core/wget",
					"github.com/u-root/u-root/cmds/exp/pxeserver",
				),
			}

			initramfs.ExtraFiles = []string{
				filepath.Join(d, "server.sh") + ":script.sh",
				filepath.Join(d, "hello") + ":etc/hello",
			}
			serverVM, err := qemu.Start(
				qemu.ArchUseEnvv,
				uqemu.WithUrootInitramfsT(t, initramfs),
				qemu.LogSerialByLine(qemu.DefaultPrint("server", t.Logf)),
				qemu.WithVMTimeout(60*time.Second),
				net.NewVM(WithNIC[*DeviceOptions](tt.nic), WithMAC[*DeviceOptions](tt.serverMAC)),
			)
			if err != nil {
				t.Fatalf("Failed to start server VM: %v", err)
			}
			t.Cleanup(func() {
				if err := serverVM.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = serverVM.Wait()
			})

			initramfs.ExtraFiles = []string{filepath.Join(d, "client.sh") + ":script.sh"}
			clientVM, err := qemu.Start(
				qemu.ArchUseEnvv,
				uqemu.WithUrootInitramfsT(t, initramfs),
				qemu.LogSerialByLine(qemu.DefaultPrint("client", t.Logf)),
				qemu.WithVMTimeout(60*time.Second),
				net.NewVM(WithNIC[*DeviceOptions](tt.nic), WithMAC[*DeviceOptions](tt.clientMAC)),
			)
			if err != nil {
				t.Fatalf("Failed to start client VM: %v", err)
			}
			if tt.clientMAC != nil {
				// Output of `ip a`
				if _, err := clientVM.Console.ExpectString(tt.clientMAC.String()); err != nil {
					t.Fatal(err)
				}
			}
			// Output of `cat ./hello`
			if _, err := clientVM.Console.ExpectString("all hello all world"); err != nil {
				t.Fatal(err)
			}
			// Output of `ls -l /sys/class/net/eth0/device/driver`
			if _, err := clientVM.Console.ExpectString("/drivers/" + tt.wantDriver); err != nil {
				t.Fatal(err)
			}
			if err := clientVM.Wait(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserIPv4(t *testing.T) {
	fs := fstest.MapFS{
		"hello": &fstest.MapFile{
			Data:    []byte("all hello all world\n"),
			Mode:    0o777,
			ModTime: time.Now(),
		},
	}

	// Serve HTTP on the host on a random port.
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(fs)))

	for _, tt := range []struct {
		nic        NIC
		clientMAC  net.HardwareAddr
		wantDriver string
	}{
		{
			nic:        NICE1000,
			wantDriver: "e1000",
		},
		{
			nic:        NICE1000,
			clientMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 1},
			wantDriver: "e1000",
		},
		{
			nic:        NICVirtioNet,
			clientMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 1},
			wantDriver: "virtio_net",
		},
	} {
		t.Run(fmt.Sprintf("%s-%s", tt.nic, tt.clientMAC), func(t *testing.T) {
			ln, err := net.Listen("tcp", ":0")
			if err != nil {
				t.Fatal(err)
			}
			port := ln.Addr().(*net.TCPAddr).Port

			clientScript := `
set -x
ip addr add 192.168.0.10/24 dev eth0
ip link set eth0 up
ip a
wget http://192.168.0.2:%d/hello
cat ./hello
ls -l /sys/class/net/eth0/device/driver
shutdown
`

			d := t.TempDir()
			_ = os.WriteFile(filepath.Join(d, "client.sh"), []byte(fmt.Sprintf(clientScript, port)), 0o777)

			s := &http.Server{
				Handler: mux,
			}
			initramfs := uroot.Opts{
				InitCmd:   "init",
				UinitCmd:  "gosh",
				UinitArgs: []string{"script.sh"},
				TempDir:   t.TempDir(),
				Commands: uroot.BusyBoxCmds(
					"github.com/u-root/u-root/cmds/core/cat",
					"github.com/u-root/u-root/cmds/core/gosh",
					"github.com/u-root/u-root/cmds/core/init",
					"github.com/u-root/u-root/cmds/core/ip",
					"github.com/u-root/u-root/cmds/core/ls",
					"github.com/u-root/u-root/cmds/core/shutdown",
					"github.com/u-root/u-root/cmds/core/wget",
				),
				ExtraFiles: []string{filepath.Join(d, "client.sh") + ":script.sh"},
			}
			vm, err := qemu.Start(
				qemu.ArchUseEnvv,
				uqemu.WithUrootInitramfsT(t, initramfs),
				qemu.LogSerialByLine(qemu.DefaultPrint("vm", t.Logf)),
				qemu.WithVMTimeout(60*time.Second),
				qemu.ServeHTTP(s, ln),
				IPv4HostNetwork("192.168.0.0/24", WithNIC[*UserOptions](tt.nic), WithMAC[*UserOptions](tt.clientMAC), WithUserArg("domainname=osfw.dev")),
			)
			if err != nil {
				t.Fatalf("Failed to start client VM: %v", err)
			}
			if tt.clientMAC != nil {
				// Output of `ip a`
				if _, err := vm.Console.ExpectString(tt.clientMAC.String()); err != nil {
					t.Fatal(err)
				}
			}
			// Output of `cat ./hello`
			if _, err := vm.Console.ExpectString("all hello all world"); err != nil {
				t.Fatal(err)
			}
			// Output of `ls -l /sys/class/net/eth0/device/driver`
			if _, err := vm.Console.ExpectString("/drivers/" + tt.wantDriver); err != nil {
				t.Fatal(err)
			}
			if err := vm.Wait(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPCAP(t *testing.T) {
	fs := fstest.MapFS{
		"hello": &fstest.MapFile{
			Data:    []byte("all hello all world\n"),
			Mode:    0o777,
			ModTime: time.Now(),
		},
	}

	// Serve HTTP on the host on a random port.
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(fs)))
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	clientScript := `
set -x
ip addr add 192.168.0.10/24 dev eth0
ip link set eth0 up
wget http://192.168.0.2:%d/hello
cat ./hello
shutdown
`
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "client.sh"), []byte(fmt.Sprintf(clientScript, port)), 0o777)

	s := &http.Server{
		Handler: mux,
	}
	initramfs := uroot.Opts{
		InitCmd:   "init",
		UinitCmd:  "gosh",
		UinitArgs: []string{"script.sh"},
		TempDir:   t.TempDir(),
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/cat",
			"github.com/u-root/u-root/cmds/core/gosh",
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/u-root/u-root/cmds/core/ip",
			"github.com/u-root/u-root/cmds/core/shutdown",
			"github.com/u-root/u-root/cmds/core/wget",
		),
		ExtraFiles: []string{filepath.Join(d, "client.sh") + ":script.sh"},
	}
	pcap := filepath.Join(t.TempDir(), "out.pcap")
	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		uqemu.WithUrootInitramfsT(t, initramfs),
		qemu.LogSerialByLine(qemu.DefaultPrint("vm", t.Logf)),
		qemu.WithVMTimeout(60*time.Second),
		qemu.ServeHTTP(s, ln),
		IPv4HostNetwork("192.168.0.0/24", WithPCAP[*UserOptions](pcap)),
	)
	if err != nil {
		t.Fatalf("Failed to start client VM: %v", err)
	}
	// Output of `cat ./hello`
	if _, err := vm.Console.ExpectString("all hello all world"); err != nil {
		t.Fatal(err)
	}
	if err := vm.Wait(); err != nil {
		t.Fatal(err)
	}

	if fi, err := os.Stat(pcap); err != nil {
		t.Fatal(err)
	} else if fi.Size() == 0 {
		t.Fatalf("PCAP file is empty")
	}
}
