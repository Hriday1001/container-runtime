package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"strings"
)

func main() {
	printCgroup()

	switch os.Args[1] {
	case "daemon":
		daemon()
	case "run":
		run()
	case "child":
		child()
	case "overlay":
		overlay()
	default:
		panic("Invalid command")
	}
}

func daemon() {
	exe, _ := os.Executable()

	unitName := "container-" + fmt.Sprint(os.Getpid())

	args := append([]string{
		"--user",
		"--scope",
		"--unit=" + unitName,
		"--collect",
		"--quiet",
		exe,
	}, append([]string{"run"}, os.Args[2:]...)...)

	cmd := exec.Command("systemd-run", args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func run() {
	fmt.Printf("Running command %v with id %d\n", os.Args[2:], os.Getpid())

	scopeName := "container-" + fmt.Sprint(os.Getppid()) + ".scope"
	controlGroups(scopeName)

	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      1000,
				Size:        1,
			},
			// {
			// 	ContainerID: 42,
			// 	HostID: 100000,
			// 	Size: 1,
			// },
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      1000,
				Size:        1,
			},
			// {
			// 	ContainerID: 42,
			// 	HostID: 100000,
			// 	Size: 1,
			// },
		},
		Unshareflags: syscall.CLONE_NEWNS,
	}
	printCgroup()
	err := cmd.Start()
	if err != nil {
		panic(err)
	}

	pid := cmd.Process.Pid

	// err_id_mappings := setUIDGIDMap(pid)
	// if err_id_mappings != nil {
	// 	panic(err_id_mappings)
	// }

	netsetgoPath := "/usr/local/bin/netsetgo"
	netsetgoCmd := exec.Command(netsetgoPath, "-pid", strconv.Itoa(pid))
	err_netsetgo := netsetgoCmd.Run()
	if err_netsetgo != nil {
		panic(err_netsetgo)
	}

	err_wait := cmd.Wait()
	if err_wait != nil {
		panic(err_wait)
	}
}

func child() {
	err_network := waitForNetworkInterfaces()
	if err_network != nil {
		panic(err_network)
	}
	getNetInterfaces()
	defer syscall.Unmount("/proc", 0)
	fmt.Printf("Running command %v with id %d\n", os.Args[2:], os.Getpid())
	// controlGroups()
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	syscall.Sethostname([]byte("container"))
	syscall.Chroot("/tmp/overlay/merged")
	syscall.Chdir("/")
	syscall.Mount("proc", "/proc", "proc", 0, "")
	err := cmd.Run()
	printCgroup()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Process exited with code %d\n", exitErr.ExitCode())
		} else {
			panic(err)
		}
	}
}

func controlGroups(unitName string) {
	printCgroup()
	cgroup_path := filepath.Join("/sys/fs/cgroup/user.slice/user-1000.slice/user@1000.service/app.slice", unitName)

	err_w := os.WriteFile(filepath.Join(cgroup_path, "pids.max"), []byte("20"), 0700)
	if err_w != nil {
		panic(err_w)
	}
}

func printCgroup() {
	pid := os.Getpid()
	cgroup_file := filepath.Join("/proc", strconv.Itoa(pid), "cgroup")
	data, err := os.ReadFile(cgroup_file)
	if err != nil {
		fmt.Printf("Error reading cgroup: %v\n", err)
		return
	}
	fmt.Printf("PID: %d, Cgroup:\n%s\n", pid, string(data))
}

func getNetInterfaces() {
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("Net interface error %v", err)
	}
	fmt.Printf("Net interfaces = %v", interfaces)
}

func waitForNetworkInterfaces() error {
	timeout := time.Second * 3
	checkInterval := time.Second
	start := time.Now()

	for {
		interfaces, err := net.Interfaces()
		if err != nil {
			panic(err)
		}

		if len(interfaces) > 1 {
			return nil
		}

		if time.Since(start) > timeout {
			return fmt.Errorf("Timeout after %s waiting for network", timeout)
		}

		time.Sleep(checkInterval)
	}
}

func setUIDGIDMap(pid int) error {
	uid := os.Getuid()
	gid := os.Getegid()

	newuidmapCmd := exec.Command("newuidmap", strconv.Itoa(pid),
		"0", strconv.Itoa(uid), "1",
		"1", "100000", "65535",
	)

	newuidmapCmd.Stdin = os.Stdin
	newuidmapCmd.Stdout = os.Stdout
	newuidmapCmd.Stderr = os.Stderr

	err_uid := newuidmapCmd.Run()
	if err_uid != nil {
		panic(err_uid)
	}

	newgidmapCmd := exec.Command("newgidmap", strconv.Itoa(pid),
		"0", strconv.Itoa(gid), "1",
		"1", "100000", "65535",
	)

	newgidmapCmd.Stdin = os.Stdin
	newgidmapCmd.Stdout = os.Stdout
	newgidmapCmd.Stderr = os.Stderr

	err_gid := newgidmapCmd.Run()
	if err_gid != nil {
		panic(err_gid)
	}

	return nil
}

func overlay() {
	tarfile := os.Args[2]
	image := strings.Split(tarfile , ".")[0]
	f, err := os.Open(tarfile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fsPath := fmt.Sprintf("./%v-fs" , image)
	if err := Untar(fsPath, f); err != nil {
		panic(err)
	}

	b, err_r := os.ReadFile(fmt.Sprintf("%v/manifest.json" , fsPath))
	if err_r != nil {
		panic(err_r)
	}
	type Image struct {
		Layers []string
	}
	var layers []Image
	e_json := json.Unmarshal(b, &layers)
	if e_json != nil {
		panic(e_json)
	}
	fmt.Printf("%+v", layers[0].Layers)
	for i, v := range layers[0].Layers {
		f, err := os.Open(fmt.Sprintf("%v/%v", fsPath , v))
		if err != nil {
			panic(err)
		}
		if err := Untar(fmt.Sprintf("%v/layer-%v", fsPath , i), f); err != nil {
			panic(err)
		}

		filepath.WalkDir(fmt.Sprintf("%v/layer-%v", fsPath , i), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				_, e := OverlayConvertWhiteout(path)
				return e
			}
			return nil
		})
	}

	abs , e  := filepath.Abs(".")
	if(e != nil){
		panic(e)
	}
	parts := make([]string, 0)
    for i := range layers[0].Layers {
        idx := len(layers[0].Layers) - i - 1
        parts = append(parts, filepath.Join(abs, fsPath, fmt.Sprintf("layer-%v", idx)))
    }

    lowerDirPath := strings.Join(parts, ":")
    fmt.Println(lowerDirPath)

	if e := mountOverlay(lowerDirPath) ; e != nil {
		panic(e)
	}
}
