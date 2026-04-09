package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func main(){
	// exe, _ := os.Executable()
	// args := append([]string{
    //     "--user",
    //     "--scope",
    //     "-p", "Delegate=yes",
    //     "--unit=container-test",
    //     exe,
    // }, append([]string{"run"} , os.Args[2:]...)...)

    // cmd := exec.Command("systemd-run", args...)
	// // cmd := exec.Command("/proc/self/exe" , append([]string{"child"} , os.Args[2:]...)...)
	// cmd.Stdin = os.Stdin;
	// cmd.Stdout = os.Stdout;
	// cmd.Stderr = os.Stderr;
	// cmd.Run()
	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child()
	default:
		panic("Invalid command")
	}
}

func run()  {
	fmt.Printf("Running command %v with id %d\n" , os.Args[2:] , os.Getpid())
	
	cmd := exec.Command("/proc/self/exe" , append([]string{"child"} , os.Args[2:]...)...)
	cmd.Stdin = os.Stdin;
	cmd.Stdout = os.Stdout;
	cmd.Stderr = os.Stderr;
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID: 1000,
				Size: 1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID: 1000,
				Size: 1,
			},
		},
		Unshareflags: syscall.CLONE_NEWNS,
	}
	err := cmd.Run()
	if(err != nil){
		panic(err)
	}
}

func child()  {
	defer syscall.Unmount("/proc" , 0)
	fmt.Printf("Running command %v with id %d\n" , os.Args[2:] , os.Getpid())
	// controlGroups()
	cmd := exec.Command(os.Args[2] , os.Args[3:]...)
	cmd.Stdin = os.Stdin;
	cmd.Stdout = os.Stdout;
	cmd.Stderr = os.Stderr;
	syscall.Sethostname([]byte("container"))
	syscall.Chroot("./image-fs")
	syscall.Chdir("/")
	syscall.Mount("proc" , "/proc" , "proc" , 0 , "")
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Process exited with code %d\n", exitErr.ExitCode())
		} else {
			panic(err)
		}
	}
}

func controlGroups() string{
	// containerID := "1234"
	// cgroup_path := fmt.Sprintf("/sys/fs/cgroup/user.slice/user-1000.slice/%v" , containerID);
	cgroup_path := "/sys/fs/cgroup/user.slice/user-1000.slice/user@1000.service/app.slice/run-r9a6a1799a09d48ea97e5e17d5c8a1229.scope"
	// err := os.Mkdir(cgroup_path , 0755)
	// if err != nil && !os.IsExist(err) {
	// 	panic(err)
	// }
	os.WriteFile(filepath.Join(cgroup_path , "pids.max") , []byte("30") , 0700)
	err_w := os.WriteFile(filepath.Join(cgroup_path , "cgroup.procs") , []byte(strconv.Itoa(os.Getpid())) , 0700)
	if err_w != nil {
		panic(err_w)
	}
	return cgroup_path
}