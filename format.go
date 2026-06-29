package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// formatAction 描述一次格式化操作
type formatAction struct {
	srcPath string // 源文件完整路径
	dstPath string // 目标路径（目录或文件）
	action  string // "extract", "copy", "delete", "skip", "mkdir"
	modType string // MOD 类型分类
	version string // 规范化后的版本号
	modName string // MOD 名称
}

// formatStats 格式化统计
type formatStats struct {
	extracted int
	copied    int
	deleted   int
	skipped   int
	errors    []string
}

// formatAuthorMods 根据作者名分发到对应的格式化函数
func formatAuthorMods(baseDir, author string) error {
	switch author {
	case "高数羽衫":
		return formatGaoshuYushan(baseDir)
	case "林秋鲑鱼":
		return formatLinQiuyu(baseDir)
	case "慕容孤晴":
		return formatMurongGuqing(baseDir)
	case "梧萱梦汐":
		return formatWuxuanMengxi(baseDir)
	default:
		return fmt.Errorf("不支持格式化作者: %s", author)
	}
}

// dryRunFormat 预览格式化操作（不实际执行）
func dryRunFormat(baseDir, author string) error {
	switch author {
	case "高数羽衫":
		return dryRunGaoshuYushan(baseDir)
	case "林秋鲑鱼":
		return dryRunLinQiuyu(baseDir)
	case "慕容孤晴":
		return dryRunMurongGuqing(baseDir)
	case "梧萱梦汐":
		return dryRunWuxuanMengxi(baseDir)
	default:
		return fmt.Errorf("不支持格式化作者: %s", author)
	}
}

// formatGaoshuYushan 格式化高数羽衫的 mod 文件夹
func formatGaoshuYushan(baseDir string) error {
	author := "高数羽衫"
	srcAuthor := filepath.Join(baseDir, author)
	dstAuthor := filepath.Join(baseDir, author) // 原地格式化

	// 检查源目录是否存在
	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	var actions []formatAction

	// 收集所有格式化操作
	actions = append(actions, collectModifier(srcAuthor)...)
	actions = append(actions, collectPlants(srcAuthor)...)
	actions = append(actions, collectZombies(srcAuthor)...)
	actions = append(actions, collectPets(srcAuthor)...)
	actions = append(actions, collectSniperDLC(srcAuthor)...)
	actions = append(actions, collectFusionMod(srcAuthor)...)
	actions = append(actions, collectSkins(srcAuthor)...)
	actions = append(actions, collectAnniversary(srcAuthor)...)
	actions = append(actions, collectRootFiles(srcAuthor)...)

	// 去重：按 (srcPath, action) 分组，避免同一源文件被处理两次
	seen := make(map[string]bool)
	var deduped []formatAction
	for _, a := range actions {
		key := a.srcPath + "|" + a.action
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, a)
		}
	}
	actions = deduped

	// 执行操作
	stats := &formatStats{}
	for _, a := range actions {
		if err := executeAction(a, dstAuthor, stats); err != nil {
			stats.errors = append(stats.errors, fmt.Sprintf("%s: %v", a.srcPath, err))
		}
	}

	// 清理空的源目录
	cleanupEmptyDirs(srcAuthor)

	// 打印统计
	fmt.Printf("\n=== %s 格式化完成 ===\n", author)
	fmt.Printf("解压: %d\n", stats.extracted)
	fmt.Printf("复制: %d\n", stats.copied)
	fmt.Printf("删除: %d\n", stats.deleted)
	fmt.Printf("跳过: %d\n", stats.skipped)
	if len(stats.errors) > 0 {
		fmt.Printf("错误: %d\n", len(stats.errors))
		for _, e := range stats.errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}

// ============================================================
// 版本号处理
// ============================================================

// versionMap 将原始版本目录名映射到规范化版本
var versionMap = map[string]string{
	"3.6（兼容3.6.1）": "3.6.1",
	"3.6.1":        "3.6.1",
	"3.7":          "3.7",
}

// normalizeVersion 规范化版本号
// 返回空字符串表示该版本应被删除（3.6 之前的版本）
func normalizeVersion(raw string) string {
	if v, ok := versionMap[raw]; ok {
		return v
	}
	// 检测是否为 3.6 之前的版本号（如 2.3, 2.4, 3.1.1 等）
	if isPre36(raw) {
		return "" // 信号：删除
	}
	return raw
}

// isPre36 判断版本号是否小于 3.6
func isPre36(v string) bool {
	v = strings.TrimSpace(v)
	// 匹配如 "2.3", "2.4", "2.7", "3.1.1" 等
	re := regexp.MustCompile(`^(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(v)
	if len(matches) != 3 {
		return false
	}
	major := matches[1]
	minor := matches[2]
	// 主版本 < 3，或主版本 = 3 且次版本 < 6
	if major < "3" {
		return true
	}
	if major == "3" && minor < "6" {
		return true
	}
	return false
}

// versionFromFilename 从文件名中提取版本号
// 如 "【融合】更多的狙击拓展2.7.exe" → "2.7"
func versionFromFilename(name string) string {
	re := regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)
	matches := re.FindAllString(name, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1] // 取最后一个版本号
	}
	return ""
}

// ============================================================
// 通用文件操作
// ============================================================

// targetDir 构造目标目录路径
func targetDir(author, version, modType, modName string) string {
	return filepath.Join(
		fmt.Sprintf("%s-%s", author, version),
		modType,
		modName,
	)
}

// zipNameToModName 从 zip 文件名提取 MOD 名称（去掉扩展名和前缀）
func zipNameToModName(zipName string) string {
	name := strings.TrimSuffix(zipName, ".zip")
	name = strings.TrimSuffix(name, ".ZIP")
	// 去掉 "僵尸-" 前缀
	name = strings.TrimPrefix(name, "僵尸-")
	return name
}

// extractZip 解压 zip 文件到目标目录
func extractZip(zipPath, dstDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	for _, f := range reader.File {
		targetPath := filepath.Join(dstDir, f.Name)

		// 防止 zip slip 攻击
		absDst, _ := filepath.Abs(dstDir)
		absTarget, _ := filepath.Abs(targetPath)
		if !strings.HasPrefix(absTarget, absDst) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}

		// 确保父目录存在
		os.MkdirAll(filepath.Dir(targetPath), 0755)

		srcFile, err := f.Open()
		if err != nil {
			return err
		}

		dstFile, err := os.Create(targetPath)
		if err != nil {
			srcFile.Close()
			return err
		}

		_, err = io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// moveFile 移动文件（复制后删除源文件）
func moveFile(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// executeAction 执行单个格式化操作
func executeAction(a formatAction, baseDir string, stats *formatStats) error {
	switch a.action {
	case "extract":
		dstDir := filepath.Join(baseDir, a.dstPath)
		fmt.Printf("  解压: %s → %s\n", a.srcPath, dstDir)
		if err := extractZip(a.srcPath, dstDir); err != nil {
			return err
		}
		// 解压后删除源 zip 文件
		os.Remove(a.srcPath)
		stats.extracted++
		return nil

	case "copy":
		dstFile := filepath.Join(baseDir, a.dstPath)
		fmt.Printf("  复制: %s → %s\n", a.srcPath, dstFile)
		if err := moveFile(a.srcPath, dstFile); err != nil {
			return err
		}
		stats.copied++
		return nil

	case "move_zip":
		// 不解压，将 zip 文件整体移动到目标目录
		dstFile := filepath.Join(baseDir, a.dstPath, filepath.Base(a.srcPath))
		fmt.Printf("  移动: %s → %s\n", a.srcPath, dstFile)
		if err := moveFile(a.srcPath, dstFile); err != nil {
			return err
		}
		stats.copied++
		return nil

	case "move_exe":
		// 不解压，将 exe 文件直接移动到目标目录
		dstFile := filepath.Join(baseDir, a.dstPath, filepath.Base(a.srcPath))
		fmt.Printf("  移动: %s → %s\n", a.srcPath, dstFile)
		if err := moveFile(a.srcPath, dstFile); err != nil {
			return err
		}
		stats.copied++
		return nil

	case "delete":
		fmt.Printf("  删除: %s\n", a.srcPath)
		if err := os.RemoveAll(a.srcPath); err != nil {
			return err
		}
		stats.deleted++
		return nil

	case "skip":
		stats.skipped++
		return nil

	case "mkdir":
		// 目录已在上层创建
		return nil

	default:
		return fmt.Errorf("未知操作: %s", a.action)
	}
}

// cleanupOldSourceDirs 删除格式化后残留的旧源目录
// 保留以 "{author}-" 开头的目标目录，删除其余目录和文件
func cleanupOldSourceDirs(srcAuthor, author string) {
	prefix := author + "-"
	entries, _ := os.ReadDir(srcAuthor)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			continue // 保留目标目录
		}
		os.RemoveAll(filepath.Join(srcAuthor, e.Name()))
	}
}

// cleanupEmptyDirs 递归删除空目录
func cleanupEmptyDirs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(dir, entry.Name())
			cleanupEmptyDirs(subDir)
			// 尝试删除空目录
			subEntries, _ := os.ReadDir(subDir)
			if len(subEntries) == 0 {
				os.Remove(subDir)
			}
		}
	}
}

// ============================================================
// 一、1.修改器 → 修改器
// ============================================================

func collectModifier(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "1.修改器")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"

	verDirs, _ := os.ReadDir(srcDir)
	for _, verDir := range verDirs {
		if !verDir.IsDir() {
			continue
		}
		rawVer := verDir.Name()
		normVer := normalizeVersion(rawVer)
		if normVer == "" {
			// 版本 < 3.6 → 全部删除
			verPath := filepath.Join(srcDir, rawVer)
			actions = append(actions, formatAction{
				srcPath: verPath,
				action:  "delete",
				modType: "修改器",
				version: rawVer,
			})
			continue
		}

		verPath := filepath.Join(srcDir, rawVer)
		platformDirs, _ := os.ReadDir(verPath)
		for _, platDir := range platformDirs {
			if !platDir.IsDir() {
				continue
			}
			platPath := filepath.Join(verPath, platDir.Name())
			files, _ := os.ReadDir(platPath)
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				srcFile := filepath.Join(platPath, f.Name())
				ext := strings.ToLower(filepath.Ext(f.Name()))

				switch {
				case ext == ".apk":
					// APK 文件 → 删除
					actions = append(actions, formatAction{
						srcPath: srcFile,
						action:  "delete",
						modType: "修改器",
						version: normVer,
					})
				case ext == ".zip":
					// Zip 文件 → 不解压，移动到同名目录下
					modName := zipNameToModName(f.Name())
					dst := targetDir(author, normVer, "修改器", modName)
					actions = append(actions, formatAction{
						srcPath: srcFile,
						dstPath: dst,
						action:  "move_zip",
						modType: "修改器",
						version: normVer,
						modName: modName,
					})
				default:
					// exe 等其他文件 → 不解压，直接移动
					dst := targetDir(author, normVer, "修改器", "")
					actions = append(actions, formatAction{
						srcPath: srcFile,
						dstPath: dst,
						action:  "move_exe",
						modType: "修改器",
						version: normVer,
						modName: f.Name(),
					})
				}
			}
		}
	}

	return actions
}

// ============================================================
// 二、2.二创植物 → 植物MOD
// ============================================================

func collectPlants(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "2.二创植物")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"

	verDirs, _ := os.ReadDir(srcDir)
	for _, verDir := range verDirs {
		if !verDir.IsDir() {
			continue
		}
		rawVer := verDir.Name()
		normVer := normalizeVersion(rawVer)
		if normVer == "" {
			verPath := filepath.Join(srcDir, rawVer)
			actions = append(actions, formatAction{
				srcPath: verPath,
				action:  "delete",
				modType: "植物MOD",
				version: rawVer,
			})
			continue
		}

		verPath := filepath.Join(srcDir, rawVer)
		files, _ := os.ReadDir(verPath)
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
				continue
			}
			srcFile := filepath.Join(verPath, f.Name())
			modName := zipNameToModName(f.Name())
			dst := targetDir(author, normVer, "植物MOD", modName)

			actions = append(actions, formatAction{
				srcPath: srcFile,
				dstPath: dst,
				action:  "extract",
				modType: "植物MOD",
				version: normVer,
				modName: modName,
			})
		}
	}

	return actions
}

// ============================================================
// 三、3.二创僵尸 → 僵尸MOD
// ============================================================

func collectZombies(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "3.二创僵尸")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"

	verDirs, _ := os.ReadDir(srcDir)
	for _, verDir := range verDirs {
		if !verDir.IsDir() {
			continue
		}
		rawVer := verDir.Name()
		normVer := normalizeVersion(rawVer)
		if normVer == "" {
			verPath := filepath.Join(srcDir, rawVer)
			actions = append(actions, formatAction{
				srcPath: verPath,
				action:  "delete",
				modType: "僵尸MOD",
				version: rawVer,
			})
			continue
		}

		verPath := filepath.Join(srcDir, rawVer)
		files, _ := os.ReadDir(verPath)
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
				continue
			}
			srcFile := filepath.Join(verPath, f.Name())
			modName := zipNameToModName(f.Name()) // 自动去除 "僵尸-" 前缀
			dst := targetDir(author, normVer, "僵尸MOD", modName)

			actions = append(actions, formatAction{
				srcPath: srcFile,
				dstPath: dst,
				action:  "extract",
				modType: "僵尸MOD",
				version: normVer,
				modName: modName,
			})
		}
	}

	return actions
}

// ============================================================
// 四、4.二创小宠物 → 植物MOD（通用版本）
// ============================================================

func collectPets(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "4.二创小宠物")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"
	normVer := "通用"

	files, _ := os.ReadDir(srcDir)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
			continue
		}
		srcFile := filepath.Join(srcDir, f.Name())
		modName := zipNameToModName(f.Name())
		dst := targetDir(author, normVer, "植物MOD", modName)

		actions = append(actions, formatAction{
			srcPath: srcFile,
			dstPath: dst,
			action:  "extract",
			modType: "植物MOD",
			version: normVer,
			modName: modName,
		})
	}

	return actions
}

// ============================================================
// 五、狙击dlc → 全部删除
// ============================================================

func collectSniperDLC(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "狙击dlc")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}
	return []formatAction{{
		srcPath: srcDir,
		action:  "delete",
		modType: "修改器",
		version: "删除",
	}}
}

// ============================================================
// 六、融合Mod → 按子目录分类处理
// ============================================================

func collectFusionMod(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "融合Mod")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"
	normVer := "通用"

	subDirs, _ := os.ReadDir(srcDir)
	for _, sub := range subDirs {
		if !sub.IsDir() {
			continue
		}
		subPath := filepath.Join(srcDir, sub.Name())

		switch sub.Name() {
		// 6.1 关卡类 → 关卡
		case "纯水图":
			actions = append(actions, collectFusionFiles(subPath, author, normVer, "关卡", "纯水图")...)
		case "丛林图":
			actions = append(actions, collectFusionFiles(subPath, author, normVer, "关卡", "丛林图")...)
		case "关卡加载器":
			actions = append(actions, collectFusionFiles(subPath, author, normVer, "关卡", "关卡加载器")...)

		// 6.2 更多的锤子 → 删除
		case "更多的锤子":
			actions = append(actions, formatAction{
				srcPath: subPath,
				action:  "delete",
				modType: "修改器",
				version: "删除",
			})

		// 6.3 领袖僵尸调整包 → 僵尸MOD（使用目录名+zip名组合）
		case "领袖僵尸调整包":
			actions = append(actions, collectFusionFilesWithPrefix(subPath, author, normVer, "僵尸MOD", "领袖僵尸调整包")...)

		// 6.4 皮肤加载器 → 皮肤MOD
		case "皮肤加载器":
			actions = append(actions, collectFusionSkinLoader(subPath, author, normVer)...)

		// 6.5 其他融合Mod → 其他
		case "死神狙索敌补丁":
			actions = append(actions, collectFusionFiles(subPath, author, normVer, "其他", "死神狙索敌补丁")...)
		case "我给你放个":
			actions = append(actions, collectFusionFiles(subPath, author, normVer, "其他", "我给你放个")...)
		case "真正的太阳":
			actions = append(actions, collectFusionFiles(subPath, author, normVer, "其他", "真正的太阳")...)
		}
	}

	return actions
}

// collectFusionFiles 收集融合Mod子目录下的所有文件
func collectFusionFiles(subPath, author, normVer, modType, modName string) []formatAction {
	var actions []formatAction

	files, _ := os.ReadDir(subPath)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		srcFile := filepath.Join(subPath, f.Name())
		ext := strings.ToLower(filepath.Ext(f.Name()))

		if ext == ".zip" {
			dst := targetDir(author, normVer, modType, modName)
			actions = append(actions, formatAction{
				srcPath: srcFile,
				dstPath: dst,
				action:  "extract",
				modType: modType,
				version: normVer,
				modName: modName,
			})
		}
	}

	return actions
}

// collectFusionFilesWithPrefix 收集融合Mod文件，使用 dirname_zipname 作为 mod 名称
func collectFusionFilesWithPrefix(subPath, author, normVer, modType, prefix string) []formatAction {
	var actions []formatAction

	files, _ := os.ReadDir(subPath)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
			continue
		}
		srcFile := filepath.Join(subPath, f.Name())
		modName := prefix + "_" + zipNameToModName(f.Name())
		dst := targetDir(author, normVer, modType, modName)

		actions = append(actions, formatAction{
			srcPath: srcFile,
			dstPath: dst,
			action:  "extract",
			modType: modType,
			version: normVer,
			modName: modName,
		})
	}

	return actions
}

// collectFusionSkinLoader 处理皮肤加载器
func collectFusionSkinLoader(subPath, author, normVer string) []formatAction {
	var actions []formatAction

	files, _ := os.ReadDir(subPath)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
			continue
		}
		srcFile := filepath.Join(subPath, f.Name())
		modName := zipNameToModName(f.Name())
		// SkinLoader.zip → "皮肤加载器", SkinLoader.Melon.zip → "皮肤加载器_Melon"
		if modName == "SkinLoader" {
			modName = "皮肤加载器"
		} else if strings.HasPrefix(modName, "SkinLoader.") {
			suffix := strings.TrimPrefix(modName, "SkinLoader.")
			modName = "皮肤加载器_" + suffix
		}
		dst := targetDir(author, normVer, "皮肤MOD", modName)

		actions = append(actions, formatAction{
			srcPath: srcFile,
			dstPath: dst,
			action:  "extract",
			modType: "皮肤MOD",
			version: normVer,
			modName: modName,
		})
	}

	return actions
}

// ============================================================
// 七、植物皮肤 → 皮肤MOD（通用版本）
// ============================================================

func collectSkins(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "植物皮肤")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"
	normVer := "通用"

	files, _ := os.ReadDir(srcDir)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
			continue
		}
		srcFile := filepath.Join(srcDir, f.Name())
		modName := zipNameToModName(f.Name())
		dst := targetDir(author, normVer, "皮肤MOD", modName)

		actions = append(actions, formatAction{
			srcPath: srcFile,
			dstPath: dst,
			action:  "extract",
			modType: "皮肤MOD",
			version: normVer,
			modName: modName,
		})
	}

	return actions
}

// ============================================================
// 八、周年庆单品 → 植物MOD（通用版本）
// ============================================================

func collectAnniversary(srcAuthor string) []formatAction {
	srcDir := filepath.Join(srcAuthor, "周年庆单品")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	var actions []formatAction
	author := "高数羽衫"
	normVer := "通用"

	files, _ := os.ReadDir(srcDir)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
			continue
		}
		srcFile := filepath.Join(srcDir, f.Name())
		modName := zipNameToModName(f.Name())
		dst := targetDir(author, normVer, "植物MOD", modName)

		actions = append(actions, formatAction{
			srcPath: srcFile,
			dstPath: dst,
			action:  "extract",
			modType: "植物MOD",
			version: normVer,
			modName: modName,
		})
	}

	return actions
}

// ============================================================
// 九、根目录文件（BepInEx.zip）
// ============================================================

func collectRootFiles(srcAuthor string) []formatAction {
	var actions []formatAction

	entries, _ := os.ReadDir(srcAuthor)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// BepInEx.zip 跳过，其他文件保留不动
		name := entry.Name()
		if name == "BepInEx.zip" {
			srcFile := filepath.Join(srcAuthor, name)
			actions = append(actions, formatAction{
				srcPath: srcFile,
				action:  "skip",
				modType: "框架",
				version: "",
				modName: name,
			})
		}
	}

	return actions
}

// ============================================================
// 工具函数
// ============================================================

// collectAllStats 辅助函数，对 actions 按类别统计
func collectAllStats(actions []formatAction) map[string]int {
	stats := map[string]int{
		"extract": 0,
		"copy":    0,
		"delete":  0,
		"skip":    0,
	}
	for _, a := range actions {
		stats[a.action]++
	}
	return stats
}

// ============================================================
// 林秋鲑鱼 格式化
// ============================================================

// decodeZipName 解码 zip 条目名称，自动处理 GBK 编码
// ZIP 规范中 bit 11 表示 UTF-8；未设置时文件名可能为系统本地编码（如 GBK）
func decodeZipName(f *zip.File) string {
	if f.Flags&0x0800 != 0 {
		return f.Name // UTF-8 标志已设置
	}
	// 尝试 GBK 解码
	decoder := simplifiedchinese.GBK.NewDecoder()
	decoded, _, err := transform.Bytes(decoder, []byte(f.Name))
	if err != nil {
		return f.Name // 解码失败，返回原始名称
	}
	return string(decoded)
}

// lqyVersionRe 从文件名提取版本号
var lqyVersionRe = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)

// formatLinQiuyu 格式化林秋鲑鱼的 mod 文件夹
func formatLinQiuyu(baseDir string) error {
	author := "林秋鲑鱼"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	entries, err := os.ReadDir(srcAuthor)
	if err != nil {
		return err
	}

	stats := &formatStats{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") {
			continue
		}

		version := extractLQYVersion(entry.Name())
		if version == "" {
			fmt.Printf("  跳过: %s (无法提取版本号)\n", entry.Name())
			stats.skipped++
			continue
		}

		zipPath := filepath.Join(srcAuthor, entry.Name())
		fmt.Printf("处理: %s → 版本 %s\n", entry.Name(), version)
		if err := processLQYZip(zipPath, version, author, srcAuthor, stats); err != nil {
			return fmt.Errorf("处理 %s 失败: %w", entry.Name(), err)
		}

		// 删除源 zip
		os.Remove(zipPath)
	}

	fmt.Printf("\n=== %s 格式化完成 ===\n", author)
	fmt.Printf("提取文件: %d\n", stats.extracted)
	fmt.Printf("跳过: %d\n", stats.skipped)
	if len(stats.errors) > 0 {
		fmt.Printf("错误: %d\n", len(stats.errors))
		for _, e := range stats.errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}

// extractLQYVersion 从林秋鲑鱼的 zip 文件名中提取版本号
func extractLQYVersion(filename string) string {
	matches := lqyVersionRe.FindString(filename)
	return matches
}

// processLQYZip 处理单个林秋鲑鱼合集 zip
func processLQYZip(zipPath, version, author, dstBase string, stats *formatStats) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer reader.Close()

	totalEntries := 0
	skippedEntries := 0
	modCount := 0
	fileCount := 0
	seenMods := make(map[string]bool) // 记录已处理的 mod，用于统计

	for _, f := range reader.File {
		totalEntries++
		entryName := decodeZipName(f)
		category, modName, fileRel, skip := parseLQYEntry(entryName)
		if skip {
			skippedEntries++
			continue
		}

		modType := lqyCategoryByPrefix(category)
		dstDir := filepath.Join(dstBase, targetDir(author, version, modType, modName))

		if f.FileInfo().IsDir() {
			os.MkdirAll(dstDir, 0755)
			continue
		}

		// 确保目标目录存在
		os.MkdirAll(filepath.Join(dstDir, filepath.Dir(fileRel)), 0755)

		dstFile := filepath.Join(dstDir, fileRel)
		if err := extractZipEntry(f, dstFile); err != nil {
			stats.errors = append(stats.errors, fmt.Sprintf("%s: %v", f.Name, err))
			continue
		}

		fileCount++
		stats.extracted++

		modKey := version + "/" + modType + "/" + modName
		if !seenMods[modKey] {
			seenMods[modKey] = true
			modCount++
			fmt.Printf("  [%s] %s/%s\n", version, modType, modName)
		}
	}

	fmt.Printf("  → %d 个 mod, %d 个文件 (总条目: %d, 跳过: %d)\n", modCount, fileCount, totalEntries, skippedEntries)
	return nil
}

// parseLQYEntry 解析林秋鲑鱼 zip 内条目路径
// 注意: zip 文件可能使用 GBK 或 UTF-8 编码储存中文文件名,
// 因此所有匹配均使用 ASCII 前缀匹配，避免编码不兼容问题。
// 返回: category, modName, fileRelPath, skip
func parseLQYEntry(entryPath string) (category, modName, fileRel string, skip bool) {
	parts := strings.Split(entryPath, "/")

	// 找到锚点：以 "BepinEX" 开头的段
	bepIdx := -1
	for i, p := range parts {
		if strings.HasPrefix(p, "BepinEX") {
			bepIdx = i
			break
		}
	}
	if bepIdx < 0 || bepIdx+2 >= len(parts) {
		return "", "", "", true
	}

	// category = 锚点后的第一段
	category = parts[bepIdx+1]
	modType := lqyCategoryByPrefix(category)
	if modType == "" {
		return "", "", "", true
	}

	// 在 category 之后找 "BepInEx/plugins"
	afterCat := parts[bepIdx+2:]
	bepInExIdx := -1
	for i := 0; i < len(afterCat)-1; i++ {
		if afterCat[i] == "BepInEx" && afterCat[i+1] == "plugins" {
			bepInExIdx = i
			break
		}
	}
	if bepInExIdx < 0 {
		return "", "", "", true
	}

	modParts := afterCat[:bepInExIdx]
	modName = strings.Join(modParts, "/")
	fileParts := afterCat[bepInExIdx+2:]
	fileRel = strings.Join(fileParts, "/")

	// 跳过 1.AllMod集合 (编码可能为 GBK, 前缀 "1." 匹配)
	if strings.HasPrefix(category, "1.") {
		return "", "", "", true
	}
	// 跳过合集目录 (以 "All " 开头的 mod 名)
	for _, p := range modParts {
		if strings.HasPrefix(p, "All ") {
			return "", "", "", true
		}
	}

	// 去掉隐藏目录前缀：如果 modParts 有多段且首段非 ASCII
	// 如 "隐藏植物/MSalmon-外挂鲑鱼" → "MSalmon-外挂鲑鱼"
	if len(modParts) >= 2 {
		modName = strings.Join(modParts[1:], "/")
	}

	return category, modName, fileRel, false
}

// lqyCategoryByPrefix 通过 ASCII 前缀匹配分类目录名
// 兼容 GBK/UTF-8 两种编码
func lqyCategoryByPrefix(name string) string {
	prefixes := []struct {
		prefix  string
		modType string
	}{
		{"2.Mod", "植物MOD"},
		{"3.Mod", "僵尸MOD"},
		{"4.ModSkin", "皮肤MOD"},
		{"5.Mod", "关卡"},
		{"7.Mod", "其他"},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p.prefix) {
			return p.modType
		}
	}
	return ""
}

// extractZipEntry 从 zip 中提取单个文件到目标路径
func extractZipEntry(f *zip.File, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// dryRunLinQiuyu 预览林秋鲑鱼格式化
func dryRunLinQiuyu(baseDir string) error {
	author := "林秋鲑鱼"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	entries, err := os.ReadDir(srcAuthor)
	if err != nil {
		return err
	}

	totalFiles := 0
	totalMods := 0
	typeVersion := map[string]int{}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") {
			continue
		}

		version := extractLQYVersion(entry.Name())
		if version == "" {
			fmt.Printf("[跳过] %s (无法提取版本号)\n", entry.Name())
			continue
		}

		zipPath := filepath.Join(srcAuthor, entry.Name())
		fmt.Printf("预览: %s → 版本 %s\n", entry.Name(), version)

		reader, err := zip.OpenReader(zipPath)
		if err != nil {
			fmt.Printf("  错误: %v\n", err)
			continue
		}

		modCount := 0
		fileCount := 0
		seenMods := make(map[string]bool)

		for _, f := range reader.File {
			entryName := decodeZipName(f)
			category, modName, _, skip := parseLQYEntry(entryName)
			if skip || f.FileInfo().IsDir() {
				continue
			}

			fileCount++
			modType := lqyCategoryByPrefix(category)
			modKey := modType + "/" + modName
			if !seenMods[modKey] {
				seenMods[modKey] = true
				modCount++
				key := version + "/" + modType
				typeVersion[key]++
				fmt.Printf("  [%s] %s/%s\n", version, modType, modName)
			}
		}
		reader.Close()

		totalMods += modCount
		totalFiles += fileCount
		fmt.Printf("  → %d 个 mod, %d 个文件\n\n", modCount, fileCount)
	}

	fmt.Printf("=== 统计 ===\n")
	fmt.Printf("Mod 数: %d\n", totalMods)
	fmt.Printf("文件数: %d\n", totalFiles)
	fmt.Printf("Zip 数: %d\n\n", len(entries))

	fmt.Println("按版本/类型分布:")
	keys := make([]string, 0, len(typeVersion))
	for k := range typeVersion {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, typeVersion[k])
	}

	return nil
}

// ============================================================
// 慕容孤晴 格式化
// ============================================================

// formatMurongGuqing 格式化慕容孤晴的 mod 文件夹
func formatMurongGuqing(baseDir string) error {
	author := "慕容孤晴"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	actions := collectMQGActions(srcAuthor, author)

	// 去重
	seen := make(map[string]bool)
	var deduped []formatAction
	for _, a := range actions {
		key := a.srcPath + "|" + a.action
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, a)
		}
	}
	actions = deduped

	// 执行
	stats := &formatStats{}
	for _, a := range actions {
		if err := executeAction(a, srcAuthor, stats); err != nil {
			stats.errors = append(stats.errors, fmt.Sprintf("%s: %v", a.srcPath, err))
		}
	}

	cleanupOldSourceDirs(srcAuthor, author)

	fmt.Printf("\n=== %s 格式化完成 ===\n", author)
	fmt.Printf("解压: %d\n", stats.extracted)
	fmt.Printf("跳过: %d\n", stats.skipped)
	if len(stats.errors) > 0 {
		fmt.Printf("错误: %d\n", len(stats.errors))
		for _, e := range stats.errors {
			fmt.Printf("  - %s\n", e)
		}
	}
	return nil
}

// collectMQGActions 收集慕容孤晴的所有格式化操作
func collectMQGActions(srcAuthor, author string) []formatAction {
	var actions []formatAction

	verDirs, _ := os.ReadDir(srcAuthor)
	for _, verDir := range verDirs {
		if !verDir.IsDir() {
			continue
		}
		version := verDir.Name()
		verPath := filepath.Join(srcAuthor, version)

		// 找到 BepInEx二创植物 目录
		subDirs, _ := os.ReadDir(verPath)
		for _, sub := range subDirs {
			if !sub.IsDir() {
				// 跳过文件: BepInEx.zip, 使用教程.txt 等
				actions = append(actions, formatAction{
					srcPath: filepath.Join(verPath, sub.Name()),
					action:  "skip",
					version: version,
				})
				continue
			}

			subName := sub.Name()
			if !strings.Contains(subName, "BepInEx二创植物") {
				continue
			}

			// 处理二创植物目录下的所有 zip
			modDir := filepath.Join(verPath, subName)
			files, _ := os.ReadDir(modDir)
			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".zip") {
					continue
				}
				srcFile := filepath.Join(modDir, f.Name())
				modName := zipNameToModName(f.Name())
				dst := targetDir(author, version, "植物MOD", modName)

				actions = append(actions, formatAction{
					srcPath: srcFile,
					dstPath: dst,
					action:  "extract",
					modType: "植物MOD",
					version: version,
					modName: modName,
				})
			}
		}
	}

	return actions
}

// dryRunMurongGuqing 预览慕容孤晴格式化
func dryRunMurongGuqing(baseDir string) error {
	author := "慕容孤晴"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	actions := collectMQGActions(srcAuthor, author)

	actionCounts := map[string]int{}
	typeVersion := map[string]int{}

	for _, a := range actions {
		actionCounts[a.action]++
		switch a.action {
		case "skip":
			fmt.Printf("[跳过] %s\n", a.srcPath)
		case "extract":
			key := fmt.Sprintf("%s/%s", a.version, a.modType)
			typeVersion[key]++
			fmt.Printf("[解压] %s → %s\n", a.srcPath, a.dstPath)
		}
	}

	fmt.Printf("\n=== 统计 ===\n")
	fmt.Printf("提取(zip解压): %d\n", actionCounts["extract"])
	fmt.Printf("跳过:         %d\n", actionCounts["skip"])
	fmt.Printf("总计:         %d\n\n", len(actions))

	fmt.Println("按版本/类型分布:")
	keys := make([]string, 0, len(typeVersion))
	for k := range typeVersion {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, typeVersion[k])
	}

	return nil
}

// ============================================================
// 梧萱梦汐 格式化
// ============================================================

var wxmVersionRe = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)`)

// wxmCategoryMap 梧萱梦汐分类目录 → MOD 类型
var wxmCategoryMap = map[string]string{
	"MOD植物":   "植物MOD",
	"MOD僵尸":   "僵尸MOD",
	"MOD皮肤":   "皮肤MOD",
	"MOD关卡":   "关卡",
	"MOD插件":   "其他",
	"MOD铲子":   "其他",
	"MOD手套":   "其他",
	"MOD迷你宠物": "植物MOD",
	"修改器":     "修改器",
}

// wxmSkipDirs 需要跳过的目录名
var wxmSkipDirs = map[string]bool{
	"使用说明书": true,
}

// formatWuxuanMengxi 格式化梧萱梦汐的 mod 文件夹
func formatWuxuanMengxi(baseDir string) error {
	author := "梧萱梦汐"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	moved := 0
	skipped := 0
	typeVersion := map[string]int{}

	verDirs, _ := os.ReadDir(srcAuthor)
	for _, verDir := range verDirs {
		if !verDir.IsDir() {
			continue
		}

		version := wxmExtractVersion(verDir.Name())
		if version == "" {
			continue
		}
		verPath := filepath.Join(srcAuthor, verDir.Name())

		catDirs, _ := os.ReadDir(verPath)
		for _, catDir := range catDirs {
			if !catDir.IsDir() {
				continue
			}
			catName := catDir.Name()

			// 跳过前置文件目录
			if strings.HasPrefix(catName, ".【置顶】") || strings.HasPrefix(catName, "【置顶】") {
				fmt.Printf("  跳过: %s\n", catName)
				skipped++
				continue
			}

			modType, ok := wxmCategoryMap[catName]
			if !ok {
				continue
			}
			catPath := filepath.Join(verPath, catName)

			// 遍历 mod 目录
			modDirs, _ := os.ReadDir(catPath)
			for _, modDir := range modDirs {
				if !modDir.IsDir() {
					continue
				}
				modName := modDir.Name()
				modPath := filepath.Join(catPath, modName)

				// 跳过特定目录
				if wxmSkipDirs[modName] {
					fmt.Printf("  跳过: %s/%s\n", catName, modName)
					skipped++
					continue
				}

				// 检查是否有变体子目录
				subDirs := getSubDirs(modPath)
				if len(subDirs) > 0 {
					// 变体 mod: 每个子目录单独移动
					for _, sub := range subDirs {
						variantName := modName + "_" + sub.Name()
						srcSub := filepath.Join(modPath, sub.Name())
						dst := targetDir(author, version, modType, variantName)
						dstFull := filepath.Join(srcAuthor, dst)

						fmt.Printf("  移动: %s → %s\n", srcSub, dstFull)
						if err := moveDirContent(srcSub, dstFull); err != nil {
							return fmt.Errorf("移动 %s 失败: %w", srcSub, err)
						}
						moved++
						key := version + "/" + modType
						typeVersion[key]++
					}
					// 变体父目录清空后删除
					os.Remove(modPath)
				} else {
					// 普通 mod: 直接移动整个目录
					dst := targetDir(author, version, modType, modName)
					dstFull := filepath.Join(srcAuthor, dst)

					fmt.Printf("  移动: %s → %s\n", modPath, dstFull)
					if err := moveDirContent(modPath, dstFull); err != nil {
						return fmt.Errorf("移动 %s 失败: %w", modPath, err)
					}
					moved++
					key := version + "/" + modType
					typeVersion[key]++
				}
			}

			// 处理 category 目录根级别的文件（如修改器下的 .rar/.txt/.mkv、MOD皮肤下的 .rar）
			catEntries, _ := os.ReadDir(catPath)
			for _, fe := range catEntries {
				if fe.IsDir() {
					continue
				}
				srcFile := filepath.Join(catPath, fe.Name())
				dst := targetDir(author, version, modType, "")
				dstFull := filepath.Join(srcAuthor, dst, fe.Name())
				fmt.Printf("  移动: %s → %s\n", srcFile, dstFull)
				os.MkdirAll(filepath.Dir(dstFull), 0755)
				if err := os.Rename(srcFile, dstFull); err != nil {
					copyFile(srcFile, dstFull)
					os.Remove(srcFile)
				}
			}
		}
	}

	cleanupOldSourceDirs(srcAuthor, author)

	fmt.Printf("\n=== %s 格式化完成 ===\n", author)
	fmt.Printf("移动: %d\n", moved)
	fmt.Printf("跳过: %d\n", skipped)

	fmt.Println("\n按版本/类型分布:")
	keys := make([]string, 0, len(typeVersion))
	for k := range typeVersion {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, typeVersion[k])
	}

	return nil
}

// wxmExtractVersion 从长版本目录名提取版本号
// "3.6.1（新lib20260602..." → "3.6.1"
func wxmExtractVersion(name string) string {
	return wxmVersionRe.FindString(name)
}

// getSubDirs 获取目录下的所有子目录
func getSubDirs(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	return dirs
}

// moveDirContent 移动目录内容：如果目标是新目录，直接 rename；否则合并
func moveDirContent(src, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0755)

	// 如果目标不存在且源在同文件系统，直接用 os.Rename
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return os.Rename(src, dst)
	}

	// 目标已存在（合并场景），逐个移动文件
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			// rename 失败（跨分区），回退到复制+删除
			if copyErr := copyDirRecursive(srcPath, dstPath); copyErr != nil {
				return copyErr
			}
			os.RemoveAll(srcPath)
		}
	}
	return os.Remove(src)
}

// copyDirRecursive 递归复制目录
func copyDirRecursive(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// dryRunWuxuanMengxi 预览梧萱梦汐格式化
func dryRunWuxuanMengxi(baseDir string) error {
	author := "梧萱梦汐"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	moved := 0
	skipped := 0
	typeVersion := map[string]int{}

	verDirs, _ := os.ReadDir(srcAuthor)
	for _, verDir := range verDirs {
		if !verDir.IsDir() {
			continue
		}
		version := wxmExtractVersion(verDir.Name())
		if version == "" {
			continue
		}
		verPath := filepath.Join(srcAuthor, verDir.Name())

		catDirs, _ := os.ReadDir(verPath)
		for _, catDir := range catDirs {
			if !catDir.IsDir() {
				continue
			}
			catName := catDir.Name()

			if strings.HasPrefix(catName, ".【置顶】") || strings.HasPrefix(catName, "【置顶】") {
				fmt.Printf("[跳过] %s\n", catName)
				skipped++
				continue
			}

			modType, ok := wxmCategoryMap[catName]
			if !ok {
				continue
			}
			catPath := filepath.Join(verPath, catName)
			modDirs, _ := os.ReadDir(catPath)
			for _, modDir := range modDirs {
				if !modDir.IsDir() {
					continue
				}
				modName := modDir.Name()

				if wxmSkipDirs[modName] {
					fmt.Printf("[跳过] %s/%s\n", catName, modName)
					skipped++
					continue
				}

				modPath := filepath.Join(catPath, modName)
				subDirs := getSubDirs(modPath)
				if len(subDirs) > 0 {
					for _, sub := range subDirs {
						variantName := modName + "_" + sub.Name()
						fmt.Printf("[移动] %s/%s/%s → %s/%s\n", catName, modName, sub.Name(), modType, variantName)
						moved++
						key := version + "/" + modType
						typeVersion[key]++
					}
				} else {
					fmt.Printf("[移动] %s/%s → %s/%s\n", catName, modName, modType, modName)
					moved++
					key := version + "/" + modType
					typeVersion[key]++
				}
			}
		}
	}

	fmt.Printf("\n=== 统计 ===\n")
	fmt.Printf("移动: %d\n", moved)
	fmt.Printf("跳过: %d\n", skipped)
	fmt.Printf("总计: %d\n\n", moved+skipped)

	fmt.Println("按版本/类型分布:")
	keys := make([]string, 0, len(typeVersion))
	for k := range typeVersion {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, typeVersion[k])
	}

	return nil
}

// printActionSummary 打印操作摘要
func printActionSummary(actions []formatAction) {
	typeCounts := make(map[string]int)
	for _, a := range actions {
		if a.action == "delete" || a.action == "skip" {
			continue
		}
		key := fmt.Sprintf("%s | %s", a.version, a.modType)
		typeCounts[key]++
	}
	fmt.Println("\n--- 计划执行操作 ---")
	keys := make([]string, 0, len(typeCounts))
	for k := range typeCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  [%s] x%d\n", k, typeCounts[k])
	}
}

// dryRunGaoshuYushan 预览格式化高数羽衫（不执行）
func dryRunGaoshuYushan(baseDir string) error {
	author := "高数羽衫"
	srcAuthor := filepath.Join(baseDir, author)

	if _, err := os.Stat(srcAuthor); os.IsNotExist(err) {
		return fmt.Errorf("作者目录不存在: %s", srcAuthor)
	}

	var actions []formatAction
	actions = append(actions, collectModifier(srcAuthor)...)
	actions = append(actions, collectPlants(srcAuthor)...)
	actions = append(actions, collectZombies(srcAuthor)...)
	actions = append(actions, collectPets(srcAuthor)...)
	actions = append(actions, collectSniperDLC(srcAuthor)...)
	actions = append(actions, collectFusionMod(srcAuthor)...)
	actions = append(actions, collectSkins(srcAuthor)...)
	actions = append(actions, collectAnniversary(srcAuthor)...)
	actions = append(actions, collectRootFiles(srcAuthor)...)

	// 统计
	actionCounts := map[string]int{}
	typeVersion := map[string]int{}
	deleteCount := 0
	skipCount := 0

	for _, a := range actions {
		actionCounts[a.action]++
		switch a.action {
		case "delete":
			deleteCount++
			fmt.Printf("[删除] %s\n", a.srcPath)
		case "skip":
			skipCount++
			fmt.Printf("[跳过] %s\n", a.srcPath)
		case "extract":
			key := fmt.Sprintf("%s/%s", a.version, a.modType)
			typeVersion[key]++
			fmt.Printf("[解压] %s → %s\n", a.srcPath, a.dstPath)
		case "move_zip":
			key := fmt.Sprintf("%s/%s", a.version, a.modType)
			typeVersion[key]++
			dstFile := filepath.Join(a.dstPath, filepath.Base(a.srcPath))
			fmt.Printf("[移动] %s → %s\n", a.srcPath, dstFile)
		case "move_exe":
			key := fmt.Sprintf("%s/%s", a.version, a.modType)
			typeVersion[key]++
			dstFile := filepath.Join(a.dstPath, filepath.Base(a.srcPath))
			fmt.Printf("[移动] %s → %s\n", a.srcPath, dstFile)
		case "copy":
			key := fmt.Sprintf("%s/%s", a.version, a.modType)
			typeVersion[key]++
			fmt.Printf("[复制] %s → %s\n", a.srcPath, a.dstPath)
		}
	}

	fmt.Printf("\n=== 统计 ===\n")
	fmt.Printf("提取(zip解压): %d\n", actionCounts["extract"])
	fmt.Printf("移动(zip/exe): %d\n", actionCounts["move_zip"]+actionCounts["move_exe"])
	fmt.Printf("删除:         %d\n", deleteCount)
	fmt.Printf("跳过:         %d\n", skipCount)
	fmt.Printf("总计:         %d\n\n", len(actions))

	if typeVersionCount := len(typeVersion); typeVersionCount > 0 {
		fmt.Println("按版本/类型分布:")
		keys := make([]string, 0, len(typeVersion))
		for k := range typeVersion {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s: %d\n", k, typeVersion[k])
		}
	}

	return nil
}
