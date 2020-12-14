package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var Conf Config

func init() {
	Conf = parseArguments()
}

func main() {
	fmt.Printf(
		"Args: %s %s %s %s\n",
		Conf.ApiKey,
		Conf.Source,
		Conf.Destination,
		Conf.Descriptor,
	)

	var tarFile = srcToTar(Conf.Source)
	tarFile.Close()
	fmt.Println(fmt.Sprintf("Temporary file: %s", tarFile.Name()))
}

func srcToTar(src string) os.File {
	dt := time.Now()
	tmpName := fmt.Sprintf("./tmp_" + Conf.Descriptor + dt.Format("02-01-2006_15:04:05") + ".tar.gz")
	file, err := os.Create(tmpName)
	if err != nil {
		processError(err)
	}
	writer := bufio.NewWriter(file)
	err = Tar(src, writer)
	if err != nil {
		processError(err)
	}
	writer.Flush()
	file.Close()
	return *file
}

func processError(err error) {
	flag.PrintDefaults()
	fmt.Print(err)
	os.Exit(2)
}

type Config struct {
	ApiKey      string
	Source      string
	Destination string
	Descriptor  string
}

func parseArguments() Config {
	var ApiKey = flag.String("key", "", "Your GoogleDrive API Key.")
	var Source = flag.String("src", ".", "The source folder to backup.")
	var Destination = flag.String("dest", "/backup", "The GoogleDrive backup destination folder.")
	var Descriptor = flag.String("tag", "", "The descriptor tag which will be included in the filename.")
	flag.Parse()

	return Config{
		ApiKey:      *ApiKey,
		Source:      *Source,
		Destination: *Destination,
		Descriptor:  *Descriptor,
	}
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
