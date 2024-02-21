package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Operations with FS
func safeMkdir(path string) error {
	err := os.Mkdir(path, 0775)
	if os.IsExist(err) {
		return nil
	}
	return err
}
func recursiveGetDirs(path string) ([]string, error) {
	dirs := []string{}

	entries, err := os.ReadDir(path)
	if err != nil {
		return dirs, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirs = append(dirs, entry.Name())

		subdirs, err := recursiveGetDirs(filepath.Join(path, entry.Name()))
		if err != nil {
			return dirs, err
		}

		for _, subdir := range subdirs {
			dirs = append(dirs, filepath.Join(entry.Name(), subdir))
		}
	}

	return dirs, nil
}
func recursiveGetFiles(path string) ([]string, error) {
	files := []string{}

	entries, err := os.ReadDir(path)
	if err != nil {
		return files, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
			continue
		}

		subfiles, err := recursiveGetFiles(filepath.Join(path, entry.Name()))
		if err != nil {
			return files, err
		}

		for _, subfile := range subfiles {
			files = append(files, filepath.Join(entry.Name(), subfile))
		}
	}

	return files, nil
}
func recursiveCopyDir(src, rmt string) error {
	err := safeMkdir(rmt)
	if err != nil {
		return err
	}

	dirs, err := recursiveGetDirs(src)
	if err != nil {
		return err
	}

	for _, dir := range dirs {
		err = safeMkdir(filepath.Join(rmt, dir))
		if err != nil {
			return err
		}
	}

	return nil
}

// Template context
func NewTemplateContext() *TemplateContext {
	envs := make(map[string]string)
	for _, str := range os.Environ() {
		substrs := strings.SplitN(str, "=", 2)
		envs[substrs[0]] = strings.Trim(substrs[1],"\n")
	}

	return &TemplateContext{
		envs: envs,
	}
}

type TemplateContext struct {
	envs map[string]string
}

// required environment variable
func (tx *TemplateContext) Env(name string) (string, error) {
	v, ok := tx.envs[name]
	if !ok {
		return "", fmt.Errorf("Error, missing variable '%v'", name)
	}
	return v, nil
}
func (tx *TemplateContext) List(name string, delimiter string) ([]string, error) {
    env, err := tx.Env(name)
    if err != nil {
        return nil, err
    }
	return strings.Split(env, delimiter), nil
}
func (tx *TemplateContext) Dict(name, itemDelimeter, kvDelimeter string) (map[string]string, error) {
    env, err := tx.Env(name)
    if err != nil {
        return nil, err
    }
	dict := map[string]string{}
	for _, substr := range strings.Split(env, itemDelimeter) {
		v := strings.SplitN(substr, kvDelimeter, 2)
		dict[v[0]] = v[1]
	}
	return dict, nil
}
func (tx *TemplateContext) Exist(name string) bool {
    _, exist := tx.envs[name]
    return exist
}
func (tx *TemplateContext) NotExist(name string) bool {
    _, exist := tx.envs[name]
    return !exist
}

// Template file
func NewTemplateFile(tx *TemplateContext, inputPath, outputPath string) *TemplateFile {
	return &TemplateFile{
		InputPath:       inputPath,
		OutputPath:      outputPath,
		TemplateContext: tx,
	}
}

type TemplateFile struct {
	InputPath       string
	Input           string
	OutputPath      string
	Output          string
	TemplateContext *TemplateContext
}

func (tf *TemplateFile) LoadInput() error {
	b, err := os.ReadFile(tf.InputPath)
	if err != nil {
		return err
	}
	tf.Input = string(b)
	return nil
}
func (tf *TemplateFile) Template() error {
	buf := new(bytes.Buffer)
	templater, err := template.New(tf.InputPath).Parse(tf.Input)
	if err != nil {
		return err
	}
	err = templater.Execute(buf, tf.TemplateContext)
	if err != nil {
		return err
	}
	tf.Output = buf.String()
	return nil
}
func (tf *TemplateFile) SaveOutput() error {
	return os.WriteFile(tf.OutputPath, []byte(tf.Output), 0664)
}

// Flags

func NewFlags() (Flags, error) {
	flags := Flags{}

	flagSet := flag.NewFlagSet("envtemplater", flag.ContinueOnError)
	flagSet.StringVar(&flags.IF, "if", "", "Input file")
	flagSet.StringVar(&flags.OF, "of", "", "Output file")
	flagSet.StringVar(&flags.ID, "id", "", "Input dir")
	flagSet.StringVar(&flags.OD, "od", "", "Output dir")

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		return flags, err
	}

	// validate
	switch {
	case flags.IF == "" && flags.ID == "":
		err = errors.New("Required input file or input dir")
	case flags.IF != "" && flags.OF == "":
		err = errors.New("Required output file when using input file")
	case flags.ID != "" && flags.OD == "":
		err = errors.New("Required output dir when using input dir")
	}

	return flags, err
}

type Flags struct {
	IF string
	OF string
	ID string
	OD string
}

func Run(flags Flags) error {
	var err error

	if flags.ID != "" {
		err = recursiveCopyDir(flags.ID, flags.OD)
		if err != nil {
			return err
		}
	}

	tx := NewTemplateContext()

	templateFiles := []*TemplateFile{}
	if flags.ID != "" {
		files, err := recursiveGetFiles(flags.ID)
		if err != nil {
			return err
		}
		for _, file := range files {
			templateFiles = append(templateFiles, NewTemplateFile(
				tx,
				filepath.Join(flags.ID, file),
				filepath.Join(flags.OD, file),
			))
		}
	} else {
		templateFiles = append(templateFiles, NewTemplateFile(
			tx,
			flags.IF,
			flags.OF,
		))
	}

	for _, templateFile := range templateFiles {
		err := templateFile.LoadInput()
		if err != nil {
			return err
		}
	}

	for _, templateFile := range templateFiles {
		err := templateFile.Template()
		if err != nil {
			return err
		}
	}

	for _, templateFile := range templateFiles {
		err := templateFile.SaveOutput()
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	flags, err := NewFlags()
	if err != nil {
		log.Fatalf("Failed parse flags: %v\n", err)
	}

	err = Run(flags)
	if err != nil {
		log.Fatalf("Failed run: %v\n", err)
	}
}
