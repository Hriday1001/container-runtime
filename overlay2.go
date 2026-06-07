package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"strings"
	"golang.org/x/sys/unix"
)

const whiteoutOpaqueDir = ".wh..wh..opq"
const whiteoutPrefix = ".wh."

func mountOverlay(lowerdir string) error{
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s" , lowerdir , "/tmp/overlay/upper" , "/tmp/overlay/work")
	e := syscall.Mount("overlay" , "/tmp/overlay/merged" , "overlay" , 0 , options)
	return e
}

func Untar(target string , r io.Reader) error{

	tr:= tar.NewReader(r)

	for {
		header , err := tr.Next()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case header == nil:
			continue
		}

		destination := filepath.Join(target , header.Name)

		switch header.Typeflag {
			case tar.TypeDir:
				_,err := os.Stat(destination);
				if err!=nil {
					err := os.MkdirAll(destination , 0755)
					if err != nil{
						return err
					}
				}
				if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
					return err
				}

			case tar.TypeReg :
				if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
					return err
				}
				f,err := os.OpenFile(destination , os.O_CREATE | os.O_WRONLY | os.O_TRUNC, os.FileMode(header.Mode))
				if err != nil {
					fmt.Println("error here : ")
					return err
				}
				
				if _,err := io.Copy(f,tr);err != nil {
					return  err
				}

				f.Close()

			case tar.TypeSymlink :
				if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
					return err
				}
				if e := os.Symlink(header.Linkname , destination); e != nil {
					return e
				}
		}
	}
}

func OverlayConvertWhiteout(path string) (bool, error) {
	base := filepath.Base(path)
	dir := filepath.Dir(path)

	if base == whiteoutOpaqueDir {
		return false, unix.Setxattr(dir, "trusted.overlay.opaque", []byte{'y'}, 0)
	}

	if strings.HasPrefix(base, whiteoutPrefix) {
		originalBase := base[len(whiteoutPrefix):]
		originalPath := filepath.Join(dir, originalBase)

		f, err := os.Create(originalPath)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		
		e := unix.Setxattr(originalPath, "trusted.overlay.whiteout", []byte{}, 0)

		return false, e
	}

	return true, nil
}
