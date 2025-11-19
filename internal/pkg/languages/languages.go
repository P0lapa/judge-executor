package languages

import "fmt"

// LanguageConfig определяет команды для компиляции и запуска
type LanguageConfig struct {
    CompileCmd []string // Команда компиляции (если нужно)
    RunCmd     []string // Команда запуска
    FileExt    string   // Расширение файла (e.g., ".cpp")
    NeedsCompile bool  // Нужно ли компилировать
}

// GetConfig возвращает config по lang
func GetConfig(lang string) (*LanguageConfig, error) {
    configs := map[string]LanguageConfig{
        "cpp": {
            CompileCmd:   []string{"g++", "-o", "exec", "code.cpp"},
            RunCmd:       []string{"./exec"},
            FileExt:      ".cpp",
            NeedsCompile: true,
        },
        "java": {
            CompileCmd:   []string{"javac", "Main.java"},
            RunCmd:       []string{"java", "Main"},
            FileExt:      ".java",
            NeedsCompile: true,
        },
        "python": {
            CompileCmd:   nil,
            RunCmd:       []string{"python3", "code.py"},
            FileExt:      ".py",
            NeedsCompile: false,
        },
        "kotlin": {
            CompileCmd:   []string{"kotlinc", "code.kt", "-include-runtime", "-d", "exec.jar"},
            RunCmd:       []string{"java", "-jar", "exec.jar"},
            FileExt:      ".kt",
            NeedsCompile: true,
        },
    }
    config, ok := configs[lang]
    if !ok {
        return nil, fmt.Errorf("unsupported language: %s", lang)
    }
    return &config, nil
}