package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModInfo 表示一个 mod 的元信息
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
	listen  string
	baseURL string
)

func main() {
	flag.StringVar(&rootDir, "dir", ".", "Mod 文件根目录")
	flag.StringVar(&listen, "listen", ":8443", "监听地址")
	flag.StringVar(&baseURL, "url", "", "基础 URL (用于生成下载链接)")
	formatAuthor := flag.String("format", "", "格式化指定作者的 mod 文件夹")
	dryRun := flag.Bool("dry-run", false, "仅预览格式化操作，不实际执行")
	flag.Parse()

	absDir, err := filepath.Abs(rootDir)
	if err != nil {
		log.Fatalf("无法解析根目录: %v", err)
	}
	rootDir = absDir

	// CLI 格式化模式
	if *formatAuthor != "" {
		if *dryRun {
			fmt.Printf("=== 预览模式 (-dry-run) ===\n")
			fmt.Printf("根目录: %s\n", rootDir)
			fmt.Printf("作者: %s\n\n", *formatAuthor)
			if err := dryRunFormat(rootDir, *formatAuthor); err != nil {
				log.Fatalf("预览失败: %v", err)
			}
			return
		}
		if err := formatAuthorMods(rootDir, *formatAuthor); err != nil {
			log.Fatalf("格式化失败: %v", err)
		}
		return
	}

	// 服务器模式
	http.HandleFunc("/api/versions", handleVersions)
	http.HandleFunc("/api/mods", handleModsList)
	http.HandleFunc("/api/formatpath", handleFormatPath)
	http.HandleFunc("/api/path", handlePathDownload)
	http.HandleFunc("/api/authors", handleAuthors)
	http.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(rootDir))))

	log.Printf("服务器启动于 http://localhost%s, 根目录: %s", listen, rootDir)
	if err := http.ListenAndServe(listen, nil); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

// ============================================================
// API 处理器
// ============================================================

func handleVersions(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	versionSet := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		authorDir := filepath.Join(rootDir, entry.Name())
		subEntries, err := os.ReadDir(authorDir)
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			ver := extractVersion(sub.Name())
			if ver != "" {
				versionSet[ver] = true
			}
		}
	}

	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	writeJSON(w, versions)
}

func handleModsList(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	verFilter := r.URL.Query().Get("ver")
	authorFilter := r.URL.Query().Get("author")
	typeFilter := r.URL.Query().Get("type")

	mods := scanMods(rootDir, verFilter, authorFilter, typeFilter)
	writeJSON(w, mods)
}

func handleFormatPath(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	author := r.URL.Query().Get("author")
	if author == "" {
		writeJSON(w, map[string]string{"status": "error", "message": "缺少 author 参数"})
		return
	}

	if err := formatAuthorMods(rootDir, author); err != nil {
		writeJSON(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}

	writeJSON(w, map[string]string{"status": "ok", "message": fmt.Sprintf("%s 格式化完成", author)})
}

func handlePathDownload(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	modPath := r.URL.Query().Get("path")
	fileName := r.URL.Query().Get("name")

	if modPath == "" {
		writeJSON(w, map[string]string{"error": "缺少 path 参数"})
		return
	}

	fullPath := filepath.Join(rootDir, modPath)
	if fileName != "" {
		fullPath = filepath.Join(fullPath, fileName)
	}

	// 安全检查：禁止目录遍历
	absRoot, _ := filepath.Abs(rootDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absRoot) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}

	if info, err := os.Stat(fullPath); err != nil || info.IsDir() {
		http.Error(w, "文件不存在", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(fullPath)))
	http.ServeFile(w, r, fullPath)
}

func handleAuthors(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	var authors []string
	for _, entry := range entries {
		if entry.IsDir() {
			authors = append(authors, entry.Name())
		}
	}
	sort.Strings(authors)
	writeJSON(w, authors)
}

// ============================================================
// 文件扫描
// ============================================================

func extractVersion(name string) string {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		return name
	}
	return ""
}

var modFileExts = map[string]bool{
	".dll": true,
	".zip": true,
	".rar": true,
	".exe": true,
	".apk": true,
}

var frameworkDlls = map[string]bool{
	"0harmony.dll":                     true,
	"asmresolver.dll":                  true,
	"bepinex.core.dll":                 true,
	"bepinex.preloader.core.dll":       true,
	"bepinex.unity.common.dll":         true,
	"bepinex.unity.il2cpp.dll":         true,
	"customizelib.bepinex.dll":         true,
	"il2cppinterop.common.dll":         true,
	"il2cppinterop.generator.dll":      true,
	"il2cppinterop.harmonysupport.dll": true,
	"il2cppinterop.runtime.dll":        true,
}

func scanMods(dir, verFilter, authorFilter, typeFilter string) []ModInfo {
	var mods []ModInfo

	authorEntries, err := os.ReadDir(dir)
	if err != nil {
		return mods
	}

	for _, authorEntry := range authorEntries {
		if !authorEntry.IsDir() {
			continue
		}
		author := authorEntry.Name()
		if authorFilter != "" && author != authorFilter {
			continue
		}

		authorDir := filepath.Join(dir, author)
		verEntries, err := os.ReadDir(authorDir)
		if err != nil {
			continue
		}

		for _, verEntry := range verEntries {
			if !verEntry.IsDir() {
				continue
			}
			verDirName := verEntry.Name()
			gameVer := extractVersion(verDirName)
			if gameVer == "" {
				continue
			}
			if verFilter != "" && gameVer != verFilter {
				continue
			}

			verDir := filepath.Join(authorDir, verDirName)
			modBestFile := make(map[string]ModInfo)

			filepath.Walk(verDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}

				ext := strings.ToLower(filepath.Ext(info.Name()))
				if !modFileExts[ext] {
					return nil
				}

				lowerName := strings.ToLower(info.Name())
				if frameworkDlls[lowerName] {
					return nil
				}

				relPath, _ := filepath.Rel(verDir, path)
				parts := strings.Split(filepath.ToSlash(relPath), "/")

				var modType, nameCN string
				fileName := info.Name()

				if len(parts) >= 1 {
					modType = parts[0]
				}
				if len(parts) >= 2 {
					nameCN = parts[1]
				}
				if nameCN == "" {
					nameCN = fileName
				}

				if typeFilter != "" && modType != typeFilter {
					return nil
				}

				depth := len(parts)
				key := modType + "/" + nameCN

				existing, exists := modBestFile[key]
				if !exists || depth < existing.depth() {
					dirPath := fmt.Sprintf("%s/%s/%s", author, verDirName, modType)
					if nameCN != "" && nameCN != fileName {
						dirPath = dirPath + "/" + nameCN
					}
					modBestFile[key] = ModInfo{
						GameVer:  gameVer,
						Author:   author,
						ModType:  modType,
						NameCN:   nameCN,
						FileName: fileName,
						URL:      fmt.Sprintf("%s/api/path?path=%s&name=%s", baseURL, dirPath, fileName),
					}
				}
				return nil
			})

			for _, m := range modBestFile {
				mods = append(mods, m)
			}
		}
	}

	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Author != mods[j].Author {
			return mods[i].Author < mods[j].Author
		}
		if mods[i].GameVer != mods[j].GameVer {
			return mods[i].GameVer < mods[j].GameVer
		}
		return mods[i].NameCN < mods[j].NameCN
	})

	return mods
}

func (m ModInfo) depth() int {
	return strings.Count(m.URL, "/")
}

// ============================================================
// HTTP 工具
// ============================================================

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
