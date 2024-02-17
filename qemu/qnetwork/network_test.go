package qnetwork

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/scriptvm"
	"github.com/u-root/mkuimage/uimage"
)

func TestInterVM(t *testing.T) {
	serverScript := `
ip addr add 192.168.0.1/24 dev eth0
ip link set eth0 up
pxeserver -4=false -http-dir=/etc
`
	clientScript := `
ip addr add 192.168.0.2/24 dev eth0
ip link set eth0 up
ip a
wget http://192.168.0.1/hello
cat ./hello
ls -l /sys/class/net/eth0/device/driver
`

	d := t.TempDir()
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
			nic:        NICVirtioNet,
			serverMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 0},
			clientMAC:  net.HardwareAddr{0xe, 0, 0, 0, 1, 1},
			wantDriver: "virtio_net",
		},
	} {
		t.Run(fmt.Sprintf("%s-%s", tt.nic, tt.serverMAC), func(t *testing.T) {
			net := NewInterVM()
			serverVM := scriptvm.Start(t, "server", serverScript,
				scriptvm.WithUimage(
					uimage.WithBusyboxCommands(
						"github.com/u-root/u-root/cmds/core/cat",
						"github.com/u-root/u-root/cmds/core/ip",
						"github.com/u-root/u-root/cmds/core/ls",
						"github.com/u-root/u-root/cmds/core/wget",
						"github.com/u-root/u-root/cmds/exp/pxeserver",
					),
					uimage.WithFiles(filepath.Join(d, "hello")+":etc/hello"),
				),
				scriptvm.WithQEMUFn(
					qemu.WithVMTimeout(90*time.Second),
					net.NewVM(WithDevice[SocketBackend](WithNIC(tt.nic), WithMAC(tt.serverMAC))),
				),
			)
			t.Cleanup(func() {
				if err := serverVM.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = serverVM.Wait()
			})

			clientVM := scriptvm.Start(t, "client", clientScript,
				scriptvm.WithUimage(
					uimage.WithBusyboxCommands(
						"github.com/u-root/u-root/cmds/core/cat",
						"github.com/u-root/u-root/cmds/core/ip",
						"github.com/u-root/u-root/cmds/core/ls",
						"github.com/u-root/u-root/cmds/core/wget",
					),
				),
				scriptvm.WithQEMUFn(
					qemu.WithVMTimeout(90*time.Second),
					net.NewVM(WithDevice[SocketBackend](WithNIC(tt.nic), WithMAC(tt.clientMAC))),
				),
			)
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

			script := fmt.Sprintf(`
				ip addr add 192.168.0.10/24 dev eth0
				ip link set eth0 up
				ip a
				wget http://192.168.0.2:%d/hello
				cat ./hello
				ls -l /sys/class/net/eth0/device/driver`, port)

			vm := scriptvm.Start(t, "vm", script,
				scriptvm.WithUimage(
					uimage.WithBusyboxCommands(
						"github.com/u-root/u-root/cmds/core/cat",
						"github.com/u-root/u-root/cmds/core/ip",
						"github.com/u-root/u-root/cmds/core/ls",
						"github.com/u-root/u-root/cmds/core/wget",
					),
				),
				scriptvm.WithQEMUFn(
					qemu.WithVMTimeout(60*time.Second),
					ServeHTTP(&http.Server{Handler: mux}, ln),
					HostNetwork("192.168.0.4/24",
						WithDevice[UserBackend](WithNIC(tt.nic), WithMAC(tt.clientMAC)),
						WithUser(WithUserArg("domainname=osfw.dev")),
					),
				),
			)
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

	script := fmt.Sprintf(`
	ip addr add 192.168.0.10/24 dev eth0
	ip link set eth0 up
	wget http://192.168.0.2:%d/hello
	cat ./hello`, port)

	pcap := filepath.Join(t.TempDir(), "out.pcap")

	vm := scriptvm.Start(t, "vm", script,
		scriptvm.WithUimage(
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/cat",
				"github.com/u-root/u-root/cmds/core/ip",
				"github.com/u-root/u-root/cmds/core/wget",
			),
		),
		scriptvm.WithQEMUFn(
			qemu.WithVMTimeout(60*time.Second),
			ServeHTTP(&http.Server{Handler: mux}, ln),
			HostNetwork("192.168.0.4/24", WithPCAP[UserBackend](pcap)),
		),
	)
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

func TestCIDRFail(t *testing.T) {
	if _, err := qemu.Start(qemu.ArchUseEnvv, HostNetwork("foobar")); err == nil {
		t.Fatalf("Expected error parsing CIDR, got nil")
	}

	if _, err := qemu.Start(qemu.ArchUseEnvv, HostNetwork("fc00::/64", WithUser(WithUserCIDR("fec0::/64")))); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Expected error parsing CIDR, got nil")
	}

	if _, err := qemu.Start(qemu.ArchUseEnvv, HostNetwork("192.168.0.0/24", WithUser(WithUserCIDR("192.169.0.0/24")))); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Expected error parsing CIDR, got nil")
	}
}

func TestUserIPv6(t *testing.T) {
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

	script := fmt.Sprintf(`
	ip addr add fec0::8/128 dev eth0
	ip link set eth0 up
	ip a
	ip -6 neigh
	ip -6 r

	# Wait for routes and neighbor requests to be there.
	# TODO: a better way to wait for the route you want to be there.
	sleep 10
	ip -6 neigh
	ip -6 r
	wget http://[fec0::2]:%d/hello
	cat ./hello
	`, port)

	vm := scriptvm.Start(t, "vm", script,
		scriptvm.WithUimage(
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/cat",
				"github.com/u-root/u-root/cmds/core/ip",
				"github.com/u-root/u-root/cmds/core/sleep",
				"github.com/u-root/u-root/cmds/core/wget",
			),
		),
		scriptvm.WithQEMUFn(
			qemu.WithVMTimeout(60*time.Second),
			ServeHTTP(&http.Server{Handler: mux}, ln),
			HostNetwork("fec0::/64"),
		),
	)
	// Output of `cat ./hello`
	if _, err := vm.Console.ExpectString("all hello all world"); err != nil {
		t.Fatal(err)
	}
	if err := vm.Wait(); err != nil {
		t.Fatal(err)
	}
}
