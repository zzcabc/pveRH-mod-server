package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ModInfo struct {
	GameVer  string `json:"game_ver"`
	Author   string `json:"author"`
	ModType  string `json:"mod_type"`
	NameCN   string `json:"name_cn"`
	NameEN   string `json:"name_en"`
	FileName string `json:"file_name"`
	URL      string `json:"url"`
}

type modEntry struct {
	ModInfo
	filePath string
}

var (
	rootDir string
	baseURL string
	listen  string
)

func main() {
	flag.StringVar(&rootDir, "dir", ".", "Mod 文件根目录")
	flag.StringVar(&listen, "listen", ":8443", "监听地址")
	flag.StringVar(&baseURL, "url", "", "基础 URL")
	flag.Parse()

	absDir, err := filepath.Abs(rootDir)
	if err != nil {
		log.Fatal("目录无效:", err)
	}
	rootDir = absDir

	http.HandleFunc("/api/versions", handleVersions)
	http.HandleFunc("/api/mods", handleModsList)
	http.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(rootDir))))

	log.Printf("Mod 服务器启动于 %s ，目录: %s\n", listen, rootDir)
	log.Fatal(http.ListenAndServe(listen, nil))
}

func handleVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			versions = append(versions, e.Name())
		}
	}
	json.NewEncoder(w).Encode(versions)
}

func handleModsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	verFilter := r.URL.Query().Get("ver")
	mods, err := scanMods(rootDir, verFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	host := baseURL
	if host == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		host = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	for i := range mods {
		rel, _ := filepath.Rel(rootDir, mods[i].filePath)
		rel = filepath.ToSlash(rel)
		mods[i].URL = fmt.Sprintf("%s/files/%s", host, rel)
		mods[i].filePath = ""
	}
	json.NewEncoder(w).Encode(mods)
}

func scanMods(dir string, verFilter string) ([]modEntry, error) {
	var mods []modEntry
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".dll") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 4 {
			return nil
		}
		gameVer := parts[0]
		if verFilter != "" && gameVer != verFilter {
			return nil
		}
		author := parts[1]
		rest := parts[2:]
		var modType, nameCN string
		if len(rest) == 2 {
			nameCN = rest[0]
		} else if len(rest) >= 3 {
			modType = rest[0]
			nameCN = rest[1]
		} else {
			return nil
		}
		fileName := info.Name()
		nameEN := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		mods = append(mods, modEntry{
			ModInfo: ModInfo{
				GameVer:  gameVer,
				Author:   author,
				ModType:  modType,
				NameCN:   nameCN,
				NameEN:   nameEN,
				FileName: fileName,
			},
			filePath: path,
		})
		return nil
	})
	return mods, err
}
