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

var envs = map[string]string {
    "dev": "http://localhost:9090",
    "staging": "https://staging.figroll.io",
    "production": "https://app.figroll.io:2113",
}

func siteMatchesAuthorization(config *Config) bool {
    var url = envs[config.Env] + "/tokens/me"

    client := &http.Client{}

    req, err := http.NewRequest("GET", url, nil)
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", config.UploadKey)

    if err != nil {
        fmt.Println("X")
        return false
    }

    resp, err := client.Do(req)
    defer resp.Body.Close()
    body, err := ioutil.ReadAll(resp.Body)

    var tokenInfo TokenInfo

    err = json.Unmarshal(body, &tokenInfo)

    if err != nil {
        fmt.Print("Error:", err)
    }

    if tokenInfo.SiteFQDN != config.SiteId {
        fmt.Println("The token does not match the site, please ensure you have the correct token")
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
    b := &bytes.Buffer{}

    fmt.Println(" > Uploading...")

    bodyWriter := multipart.NewWriter(b)

    fileWriter, err := bodyWriter.CreateFormFile("file", "upload.zip")
    if err != nil {
        return
    }

    _, err = io.Copy(fileWriter, &buf)

    bodyWriter.Close()

    // bw, err := os.Create("/tmp/dat2.txt")
    // b2 := bufio.NewWriter(bw)
    // b2.Write(b.Bytes())
    // b2.Flush()

    var uri = envs[config.Env] + "/sites/" + config.SiteId + "/upload?env=" + config.Env

    req, err := http.NewRequest("POST", uri, b)
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", config.UploadKey)
    req.Header.Add("Content-Type", bodyWriter.FormDataContentType());

    client := &http.Client{}
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

func main() {
    config := &Config{}
    if _, err := toml.DecodeFile("figroll.toml", config); err != nil {
        fmt.Println(err)
        connect()
        return
    }

    _, ok := envs[config.Env]

    if ! ok {
        fmt.Println("Env must be either 'staging' or 'production'.")
        fmt.Println(" > Deployment cancelled")
        return
    }

    fmt.Println("Deploying:", config.SiteId)
    fmt.Println(" > Target:", config.DefaultDeployEnvironment)

    // if !siteMatchesAuthorization(config) {
    //     fmt.Println(" > Your UploadKey does not match", config.SiteId)
    //     fmt.Println(" > Please get a new one from http://www.figroll.it/newkey")
    // }

    // TODO: Test if Authentication works. GET /users/me
    zipBuffer, statusCode := makeZip(config)

    if statusCode == 0 {
        fmt.Println("Uploading... please wait")
        upload(config, zipBuffer, statusCode)
    } else {
        fmt.Println("Could not zip the file")
    }
}
