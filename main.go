package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

var Conf Config

func init() {
	Conf = parseArguments()
}

func main() {
	fmt.Println("Google drive backup utility.")
	fmt.Printf(
		"Args:\n Source: %s\n Destination: %s\n Descriptor: %s\n\n",
		Conf.Source,
		Conf.Destination,
		Conf.Descriptor,
	)

	var tarFile = srcToTar(Conf.Source)
	tarFile.Close()
	fmt.Println(fmt.Sprintf("Backup Archive: %s", tarFile.Name()))

	// Step 1. Open the file
	f, err := os.Open(tarFile.Name())

	if err != nil {
		processError(err, "Cannot open backup archive.")
	}

	defer f.Close()

	// Step 2. Get the Google Drive service
	service, err := getService()

	// Step 3. Create the directory
	dir, err := createDir(service, Conf.Destination, "root")

	if err != nil {
		processError(err, "Could not create remote directory.")
	}

	// Step 4. Create the file and upload its content
	file, err := createFile(service, tarFile.Name(), "image/png", f, dir.Id)

	if err != nil {
		processError(err, fmt.Sprintf("Could not create file: %s\n", file.Name))
	}
}

func srcToTar(src string) os.File {
	dt := time.Now()
	tmpName := fmt.Sprintf("./tmp_" + Conf.Descriptor + dt.Format("02-01-2006_15:04:05") + ".tar.gz")
	file, err := os.Create(tmpName)
	if err != nil {
		processError(err, fmt.Sprintf("Could not create file: %s", tmpName))
	}
	writer := bufio.NewWriter(file)
	err = Tar(src, writer)
	if err != nil {
		processError(err, fmt.Sprintf("Error writing to file: %s", file.Name()))
	}
	writer.Flush()
	file.Close()
	return *file
}

func processError(err error, msg string) {
	flag.PrintDefaults()
	fmt.Println(fmt.Errorf("Error!\nMsg: %s\nErr: %s\n", msg, err))
	os.Exit(2)
}

type Credentials struct {
	ClientID string
	Secret   string
}

type Config struct {
	Source      string
	Destination string
	Descriptor  string
}

func parseArguments() Config {
	var Source = flag.String("src", ".", "The source folder to backup.")
	var Destination = flag.String("dest", "/backup", "The GoogleDrive backup destination folder.")
	var Descriptor = flag.String("tag", "", "The descriptor tag which will be included in the filename.")
	flag.Parse()

	return Config{
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

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		processError(err, "Unable to read authorization code")
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		processError(err, "Unable to retrieve token from web")
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		//processError(err, "")
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		processError(err, "Unable to cache oauth token.")
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getService() (*drive.Service, error) {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		processError(err, "Unable to read credentials.json file.")
		return nil, err
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)

	if err != nil {
		processError(err, "Unable to delete your previously saved token.json.")
		return nil, err
	}

	client := getClient(config)

	service, err := drive.New(client)

	if err != nil {
		processError(err, "Cannot create the Google Drive service.")
		return nil, err
	}

	return service, err
}

func createDir(service *drive.Service, name string, parentId string) (*drive.File, error) {
	d := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentId},
	}

	file, err := service.Files.Create(d).Do()

	if err != nil {
		processError(err, "Could not create dir.")
		return nil, err
	}

	return file, nil
}

func createFile(service *drive.Service, name string, mimeType string, content io.Reader, parentId string) (*drive.File, error) {
	f := &drive.File{
		MimeType: mimeType,
		Name:     name,
		Parents:  []string{parentId},
	}
	file, err := service.Files.Create(f).Media(content).Do()

	if err != nil {
		processError(err, fmt.Sprintf("Could not create file."))
		return nil, err
	}

	return file, nil
}
