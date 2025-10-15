package main

// import (
// 	"io"
// 	"log"
// 	"os"
// 	"path/filepath"
// 	"regexp"
// 	"strings"
// )

// const (
// 	CurrentDir = "." // Текущая директория проекта
// 	NewDir     = "cleaned_project"
// )

// var (
// 	// Паттерны для определения типа файла
// 	handlerPattern    = regexp.MustCompile(`(?i)(handler|api)\.go$`)
// 	modelPattern      = regexp.MustCompile(`(?i)(model|entity)\.go$`)
// 	repoPattern       = regexp.MustCompile(`(?i)repository\.go$`)
// 	servicePattern    = regexp.MustCompile(`(?i)(service|usecase)\.go$`)      // Пытаемся отличить UseCase от Service
// 	dtoPattern        = regexp.MustCompile(`(?i)(dto|request|response)\.go$`) // DTO в HTTP
// 	middlewarePattern = regexp.MustCompile(`(?i)middleware\.go$`)

// 	// Паттерны для определения доменных сущностей внутри model файлов
// 	entityPatternInFile = regexp.MustCompile(`(?m)^\s*type\s+(\w+)\s+struct\s*{`)

// 	// Папки для перемещения
// 	destinationDirs = map[string]string{
// 		"handler": filepath.Join(NewDir, "internal", "infrastructure", "api", "http", "handler"),
// 		"dto":     filepath.Join(NewDir, "internal", "infrastructure", "api", "http", "dto"),
// 		"entity":  filepath.Join(NewDir, "internal", "domain", "entity"),
// 		"repo":    filepath.Join(NewDir, "internal", "infrastructure", "database", "postgresql"), // Пока все репо в postgresql
// 		"usecase": filepath.Join(NewDir, "internal", "usecase"),
// 		"service": filepath.Join(NewDir, "internal", "domain", "service"), // Для бизнес-сервисов
// 		"config":  filepath.Join(NewDir, "internal", "config"),
// 		"util":    filepath.Join(NewDir, "internal", "util"),
// 		"other":   filepath.Join(NewDir, "internal", "infrastructure", "other"), // Для остальных
// 	}
// )

// func main() {
// 	log.Println("Starting restructuring...")

// 	// Создаем новую директорию
// 	if err := os.MkdirAll(NewDir, 0755); err != nil {
// 		log.Fatal("Failed to create new directory: ", err)
// 	}

// 	// Создаем структуру папок
// 	for _, dir := range destinationDirs {
// 		if err := os.MkdirAll(dir, 0755); err != nil {
// 			log.Fatal("Failed to create destination directory: ", err)
// 		}
// 	}

// 	// Обходим текущую структуру
// 	err := filepath.Walk(CurrentDir, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		// Пропускаем директории, новую структуру и .git
// 		if info.IsDir() || strings.HasPrefix(path, NewDir) || strings.Contains(path, ".git") {
// 			return nil
// 		}

// 		// Определяем тип файла
// 		fileType := determineFileType(path)

// 		// Выбираем директорию назначения
// 		destDir, ok := destinationDirs[fileType]
// 		if !ok {
// 			log.Printf("Unknown file type for %s, placing in 'other'", path)
// 			destDir = destinationDirs["other"]
// 		}

// 		// Формируем путь для нового файла
// 		newPath := filepath.Join(destDir, info.Name())

// 		// Проверяем на конфликты имен
// 		if _, err := os.Stat(newPath); err == nil {
// 			log.Printf("CONFLICT: File %s already exists in destination %s. Skipping.", info.Name(), destDir)
// 			return nil // Пропускаем конфликтующий файл
// 		}

// 		// Копируем файл
// 		if err := copyFile(path, newPath); err != nil {
// 			log.Printf("Failed to copy %s to %s: %v", path, newPath, err)
// 			return nil // Продолжаем, не прерываем весь процесс
// 		}

// 		log.Printf("Moved %s -> %s", path, newPath)
// 		return nil
// 	})

// 	if err != nil {
// 		log.Fatal("Error walking the path: ", err)
// 	}

// 	log.Println("Initial restructuring completed. Please review and fix imports/paths manually.")
// }

// func determineFileType(filePath string) string {
// 	baseName := filepath.Base(filePath)
// 	content, err := os.ReadFile(filePath)
// 	if err != nil {
// 		log.Printf("Error reading file %s: %v", filePath, err)
// 		return "other"
// 	}

// 	// Проверяем по имени файла
// 	if handlerPattern.MatchString(baseName) {
// 		return "handler"
// 	}
// 	if modelPattern.MatchString(baseName) {
// 		// Проверяем содержимое на предмет сущностей
// 		if entityPatternInFile.Match(content) {
// 			return "entity"
// 		}
// 		return "entity" // Для простоты, все model -> entity
// 	}
// 	if repoPattern.MatchString(baseName) {
// 		return "repo"
// 	}
// 	if servicePattern.MatchString(baseName) {
// 		// Попытка отличить UseCase от Service
// 		// Проверяем, содержит ли файл интерфейсы (порт) или вызовы репо/адаптеров
// 		// Это грубая эвристика
// 		if strings.Contains(string(content), "interface") || strings.Contains(string(content), "posRepo") || strings.Contains(string(content), "redisClient") {
// 			return "usecase"
// 		}
// 		// Если в имени есть что-то вроде "jwt", "telegram", "geo", возможно это доменный сервис
// 		if strings.Contains(baseName, "jwt") || strings.Contains(baseName, "telegram") || strings.Contains(baseName, "geo") {
// 			return "service" // Помещаем в domain/service как потенциальный бизнес-сервис
// 		}
// 		return "usecase" // По умолчанию в usecase
// 	}
// 	if dtoPattern.MatchString(baseName) {
// 		return "dto"
// 	}
// 	if middlewarePattern.MatchString(baseName) {
// 		return "util" // Мидлвары можно положить в util или отдельный пакет
// 	}

// 	// Проверяем по папке
// 	dir := filepath.Dir(filePath)
// 	switch {
// 	case strings.HasPrefix(dir, "config"):
// 		return "config"
// 	case strings.HasPrefix(dir, "handlers"):
// 		return "handler"
// 	case strings.HasPrefix(dir, "models"):
// 		return "entity"
// 	case strings.HasPrefix(dir, "repositories"):
// 		return "repo"
// 	case strings.HasPrefix(dir, "services"):
// 		// Аналогично выше, пытаемся различить
// 		if strings.Contains(baseName, "jwt") || strings.Contains(baseName, "telegram") || strings.Contains(baseName, "geo") {
// 			return "usecase" // Скорее UseCase
// 		}
// 		return "service" // Или всё-таки в domain/service?
// 	}

// 	// Проверяем по содержимому
// 	contentStr := string(content)
// 	switch {
// 	case strings.Contains(contentStr, "http.Handler") || strings.Contains(contentStr, "chi.Router"):
// 		return "handler"
// 	case strings.Contains(contentStr, "Query") || strings.Contains(contentStr, "Exec"): // Грубая проверка репозитория
// 		return "repo"
// 	case strings.Contains(contentStr, "type") && strings.Contains(contentStr, "struct"): // Грубая проверка модели
// 		return "entity"
// 	}

// 	return "other"
// }

// func copyFile(src, dst string) error {
// 	srcFile, err := os.Open(src)
// 	if err != nil {
// 		return err
// 	}
// 	defer srcFile.Close()

// 	dstFile, err := os.Create(dst)
// 	if err != nil {
// 		return err
// 	}
// 	defer dstFile.Close()

// 	// Копируем содержимое
// 	_, err = io.Copy(dstFile, srcFile)
// 	if err != nil {
// 		return err
// 	}

// 	// Копируем права доступа
// 	srcInfo, err := srcFile.Stat()
// 	if err != nil {
// 		return err
// 	}
// 	return os.Chmod(dst, srcInfo.Mode())
// }

// // updateImports - Псевдокод/план для обновления импортов
// // Это НЕ реализовано в этом скрипте, так как требует парсинга Go-кода (go/ast)
// func updateImports() {
// 	// 1. Пройти по всем .go файлам в новой структуре
// 	// 2. Для каждого файла:
// 	//    a. Прочитать его
// 	//    b. Использовать go/ast для парсинга
// 	//    c. Найти все импорты
// 	//    d. Обновить пути импортов в соответствии с новой структурой
// 	//       Например: "github.com/user/repo/handlers" -> "github.com/user/repo/internal/infrastructure/api/http/handler"
// 	//    e. Записать файл обратно
// 	//
// 	// Это сложная задача, требующая библиотеки go/ast и аккуратной обработки.
// 	// Лучше это делать вручную или с помощью рефакторинга в IDE.
// }
