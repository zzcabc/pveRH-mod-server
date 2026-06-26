package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

// extractVersion 从目录名中提取版本号。
//
//	"高数羽衫-3.6.1" → "3.6.1"
//	"3.6.1"         → "3.6.1"
//	"其他"           → ""
func extractVersion(name string) string {
	idx := strings.LastIndex(name, "-")
	if idx >= 0 {
		name = name[idx+1:]
	}
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		return name
	}
	return ""
}

func handleVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	authors, err := os.ReadDir(rootDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	versionSet := map[string]struct{}{}
	for _, author := range authors {
		if !author.IsDir() || strings.HasPrefix(author.Name(), ".") {
			continue
		}
		verDirs, err := os.ReadDir(filepath.Join(rootDir, author.Name()))
		if err != nil {
			continue
		}
		for _, vd := range verDirs {
			if !vd.IsDir() {
				continue
			}
			if ver := extractVersion(vd.Name()); ver != "" {
				versionSet[ver] = struct{}{}
			}
		}
	}
	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}
	sort.Strings(versions)
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
		mods[i].URL = fmt.Sprintf("%s/files/%s", host, mods[i].URL)
	}
	json.NewEncoder(w).Encode(mods)
}

func scanMods(dir string, verFilter string) ([]ModInfo, error) {
	var mods []ModInfo
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".dll") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		// 目录结构: {author}/{versionDir}/{modType}/{nameCN}/{file.dll}
		//            或      {author}/{versionDir}/{nameCN}/{file.dll}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 4 {
			return nil
		}
		author := parts[0]
		gameVer := extractVersion(parts[1])
		if gameVer == "" {
			return nil
		}
		if verFilter != "" && gameVer != verFilter {
			return nil
		}
		rest := parts[2:]
		var modType, nameCN string
		if len(rest) == 2 {
			nameCN = rest[0]
		} else {
			modType = rest[0]
			nameCN = rest[1]
		}
		fileName := d.Name()
		nameEN := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		mods = append(mods, ModInfo{
			GameVer:  gameVer,
			Author:   author,
			ModType:  modType,
			NameCN:   nameCN,
			NameEN:   nameEN,
			FileName: fileName,
			URL:      filepath.ToSlash(rel),
		})
		return nil
	})
	return mods, err
}
