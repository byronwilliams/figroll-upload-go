package main

import (
    "github.com/BurntSushi/toml"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "archive/zip"
    "bytes"
    "io"
    "bufio"
    "time"
    "encoding/json"
    "flag"
    "crypto/tls"

    "mime/multipart"
    "net/http"
    "log"
    "io/ioutil"
)

type Config struct {
    Env string
    SiteId string
    UploadKey string
    PublicFolder string
    DefaultDeployEnvironment string
}

type TokenInfo struct {
    ExpiresAt time.Time `json:"expires_at"`
    SiteFQDN string `json:"site_fqdn"`
}

type VersionInfo struct {
    Id string `json:"id"`
    SiteId int `json:"site_id"`
    Version int `json:"version"`
    IsActive bool `json:"is_active"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    StagingUrl string `json:"stagingUrl"`
}

const API_URL string = "https://app.figroll.io:2113"
//const API_URL string = "https://api.figroll.it:2113"
const DEBUG bool = false

func isValidAPIKey(config *Config) bool {
    var url = API_URL + "/tokens/me"

    tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: DEBUG},
    }
    client := &http.Client{Transport: tr}

    req, err := http.NewRequest("HEAD", url, nil)
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", config.UploadKey)

    if err != nil {
        return false
    }

    resp, err := client.Do(req)

    if err != nil {
        log.Println(err)
    }

    if resp.StatusCode != 200 {
        return false
    }

    return true
}

func siteMatchesAuthorization(config *Config) bool {
    var url = API_URL + "/sites/" + config.SiteId

    tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: DEBUG},
    }
    client := &http.Client{Transport: tr}

    req, err := http.NewRequest("GET", url, nil)
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", config.UploadKey)

    if err != nil {
        return false
    }

    resp, err := client.Do(req)

    if resp.StatusCode != 200 {
        return false
    }

    return true
}

func connect() {
    fmt.Println("===================================")
    fmt.Println("Connect your Static Site to Figroll")
}

func makeZip(config *Config) (bytes.Buffer, int) {
    var b bytes.Buffer

    config.PublicFolder = filepath.Clean(config.PublicFolder)
    dir, err := os.Lstat(config.PublicFolder)

    if err != nil {
        fmt.Println("Does the PublicFolder exist?")
        return b, 2
    }

    if !dir.IsDir() {
        fmt.Println("That's not a folder I can work with")
        return b, 3
    }

    fmt.Println(" > Compressing", config.PublicFolder)

    // bw, err := os.Create("/tmp/dat2.zip")
    // b := bufio.NewWriter(bw)
    zw := zip.NewWriter(&b)

    filepath.Walk(config.PublicFolder, func(path string, f os.FileInfo, err error) error {
        if f.IsDir() {
            return nil
        }

        var relPath = strings.Replace(path, config.PublicFolder + "/", "", -1)

        fmt.Println("Adding file: " + relPath)

        header, err := zip.FileInfoHeader(f)
        header.Name = "public/" + relPath

        //fmt.Println(header)

        opened_file, err := os.Open(path)
        fi := bufio.NewReader(opened_file)

        defer opened_file.Close()

        fw, err := zw.CreateHeader(header)

        // if err != nil {
        //     //log.Fatal(err)
        // }

        if _, err = io.Copy(fw, fi); err != nil {
            //log.Fatal(err)
        }

        return nil
    })

    err = zw.Close()

    return b, 0
}

func upload(config *Config, buf bytes.Buffer, status int) {
    uri := API_URL + "/sites/" + config.SiteId + "/upload?env=" + config.Env
    b := &bytes.Buffer{}

    fmt.Println(" > Uploading...")

    bodyWriter := multipart.NewWriter(b)

    fileWriter, err := bodyWriter.CreateFormFile("file", "upload.zip")
    if err != nil {
        return
    }

    _, err = io.Copy(fileWriter, &buf)

    bodyWriter.Close()

    tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: DEBUG},
    }
    client := &http.Client{Transport: tr}

    req, err := http.NewRequest("POST", uri, b)
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", config.UploadKey)
    req.Header.Add("Content-Type", bodyWriter.FormDataContentType());

    resp, err := client.Do(req)
    if err != nil {
        log.Fatal(err)
    } else {
        body, err := ioutil.ReadAll(resp.Body)

        if resp.StatusCode != 200 {
            fmt.Println(string(body))
            fmt.Println(" > Upload failed (", resp.StatusCode, ")")
            return
        }

        var vInfo VersionInfo

        err = json.Unmarshal(body, &vInfo)

        if err != nil {
            fmt.Print("Error:", err)
        }

        fmt.Println(" > Version ", vInfo.Version, " is now active")
        fmt.Printf(" > Visit %s\n", vInfo.StagingUrl)
    }
}

func Push(config *Config, env string) {
    fmt.Printf("Deploying to %s\n", env)

    if !isValidAPIKey(config) {
        fmt.Println(" > Your UploadKey is Invalid or has Expired")
        fmt.Println(" > Please get a new one from https://app.figroll.io/keys")
        return
    }

    if !siteMatchesAuthorization(config) {
        fmt.Println(" > The site with ID (", config.SiteId, ") doesn't exist")
        fmt.Println(" > You can get the ID from https://app.figroll.io/sites")
        return
    }

    // TODO: Test if Authentication works. GET /users/me
    zipBuffer, statusCode := makeZip(config)

    if statusCode == 0 {
        fmt.Println("Uploading... please wait")
        upload(config, zipBuffer, statusCode)
    } else {
        fmt.Println("Could not zip the file")
    }
}


func Usage() {
    fmt.Fprintf(os.Stderr, "Usage of %s [-conf=path]:\n", os.Args[0])
    //fmt.Fprintf(os.Stderr, "  link                    Link this system with your Figroll account\n")
    fmt.Fprintf(os.Stderr, "  push staging            Upload the site to a staging area\n")
    fmt.Fprintf(os.Stderr, "  push production         Upload the site straight to production\n")
    //fmt.Fprintf(os.Stderr, "  help                    Show this help\n")
}

func main() {
    config := &Config{}

    confFile := flag.String("conf", "figroll.toml", "Config file location")

    flag.Parse()

    args := flag.Args()

    if len(args) != 2 {
        Usage()
        return
    }

    if args[0] != "push" {
        Usage()
        return
    }

    if !(args[1] == "staging" || args[1] == "production") {
        Usage()
        return
    }

    if _, err := toml.DecodeFile(*confFile, config); err != nil {
        fmt.Println(err)
        return
    }

    Push(config, args[1])
}
