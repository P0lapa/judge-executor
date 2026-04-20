package languages

import "fmt"

type LanguageConfig struct {
	CompileCmd   []string
	RunCmd       []string
	SourceFile   string
	FileExt      string
	NeedsCompile bool
	Image        string
	IsJVM        bool
}

func GetConfig(lang string) (*LanguageConfig, error) {
	configs := map[string]LanguageConfig{
		"cpp": {
			CompileCmd:   []string{"g++", "-o", "exec", "code.cpp"},
			RunCmd:       []string{"./exec"},
			SourceFile:   "code.cpp",
			FileExt:      ".cpp",
			NeedsCompile: true,
			Image:        "judge-cpp:latest",
		},
		"java": {
			CompileCmd: []string{"javac", "Main.java"},
			RunCmd: []string{
				"java",
				"-XX:ReservedCodeCacheSize=64m",
				"-Xms32m",
				"-Xmx128m",
				"-XX:+UseSerialGC",
				"-cp", ".",
				"Main",
			},
			SourceFile:   "Main.java",
			FileExt:      ".java",
			NeedsCompile: true,
			Image:        "judge-java:latest",
			IsJVM:        true,
		},
		"python": {
			RunCmd:       []string{"python3", "code.py"},
			SourceFile:   "code.py",
			FileExt:      ".py",
			NeedsCompile: false,
			Image:        "judge-python:3.10",
		},
		"kotlin": {
			CompileCmd: []string{"kotlinc", "code.kt", "-include-runtime", "-d", "exec.jar"},
			RunCmd: []string{
				"java",
				"-XX:ReservedCodeCacheSize=64m",
				"-Xms32m",
				"-Xmx128m",
				"-XX:+UseSerialGC",
				"-jar", "exec.jar",
			},
			SourceFile:   "code.kt",
			FileExt:      ".kt",
			NeedsCompile: true,
			Image:        "judge-kotlin:17",
			IsJVM:        true,
		},
	}

	config, ok := configs[lang]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}

	return &config, nil
}
