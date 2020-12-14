package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var Conf Config
var Flags *flag.FlagSet

func init() {
	Flags = flag.NewFlagSet("Default Config", flag.ContinueOnError)

	Conf = parseArguments()
}

func main() {
	fmt.Printf(
		"Args: %s %s %s %s",
		*Conf.ApiKey,
		*Conf.Source,
		*Conf.Destination,
		*Conf.Descriptor,
	)

	//var tarFile = srcToTar(SrcFolder, )
	//fmt.Print(fmt.Sprintf("%s %s %s %s %s", string(*ApiKey), string(*SrcFolder), string(*DestFolder), string(*archiveType), string(*tag)))
	//var cwd = os.Args[0:]
	//var options = os.Args[1:]
	//var source = os.Args[2:]
	//fmt.Sprint("os.Args %s\nos.Args[0:] %s\nos.Args[1:] %s\nos.Args[2:] %s\nos.Args[3:] %s\n", os.Args, os.Args[0:], os.Args[1:], os.Args[2:], os.Args[3:])
	//fmt.Print("gdrive-backup utility.")
	//if len(flag.Arg(0)) == len("") {
	//	printHelp()
	//}
}

func srcToTar(src string) os.File {
	file, err := os.Create("./tmp.tar")
	if err != nil {
		processError(err)
	}
	writer := bufio.NewWriter(file)
	err = Tar(src, writer)
	if err != nil {
		processError(err)
	}

	return *file
}

func processError(err error) {
	Flags.PrintDefaults()
	log.Fatal(err)
	os.Exit(2)
}

type Config struct {
	ApiKey      *string
	Source      *string
	Destination *string
	Descriptor  *string
}

func parseArguments() Config {
	var conf Config
	conf.ApiKey = flag.String("key", "", "Your GoogleDrive API Key.")
	conf.Source = flag.String("src", ".", "The source folder to backup.")
	conf.Destination = flag.String("dest", "/backup", "The GoogleDrive backup destination folder.")
	conf.Descriptor = flag.String("tag", "", "The descriptor tag which will be included in the filename.")
	flag.Parse()


	//if err != nil {
	//	processError(err)
	//}
	//if conf.ApiKey == ""{
	//	processError(flag.ErrHelp)
	//}

	return conf
}

// Tar takes a source and variable writers and walks 'source' writing each file
// found to the tar writer; the purpose for accepting multiple writers is to allow
// for multiple outputs (for example a file, or md5 hash)
// Credits to https://medium.com/@skdomino
func Tar(src string, writers ...io.Writer) error {

	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("Unable to tar files - %v", err.Error())
	}

	mw := io.MultiWriter(writers...)

	gzw := gzip.NewWriter(mw)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// walk path
	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

		// return on any error
		if err != nil {
			return err
		}

		// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
		if !fi.Mode().IsRegular() {
			return nil
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, src, "", -1), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// open files for taring
		f, err := os.Open(file)
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		// manually close here after each file operation; defering would cause each file close
		// to wait until all operations have completed.
		f.Close()

		return nil
	})
}
