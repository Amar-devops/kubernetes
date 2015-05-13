/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	_ "github.com/GoogleCloudPlatform/kubernetes/pkg/api/v1"
	_ "github.com/GoogleCloudPlatform/kubernetes/pkg/api/v1beta3"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"

	"github.com/golang/glog"
)

func generateConversions(t *testing.T, version string) (bytes.Buffer, bytes.Buffer) {
	g := runtime.NewGenerator(api.Scheme.Raw())
	g.OverwritePackage(version, "")
	g.OverwritePackage("api", "newer")
	for _, knownType := range api.Scheme.KnownTypes(version) {
		if err := g.GenerateConversionsForType(version, knownType); err != nil {
			glog.Errorf("error while generating conversion functions for %v: %v", knownType, err)
		}
	}

	var functions bytes.Buffer
	functionsWriter := bufio.NewWriter(&functions)
	if err := g.WriteConversionFunctions(functionsWriter); err != nil {
		t.Fatalf("couldn't generate conversion functions: %v", err)
	}
	if err := functionsWriter.Flush(); err != nil {
		t.Fatalf("error while flushing writer")
	}

	var names bytes.Buffer
	namesWriter := bufio.NewWriter(&names)
	if err := g.WriteConversionFunctionNames(namesWriter); err != nil {
		t.Fatalf("couldn't generate conversion function names: %v", err)
	}
	if err := namesWriter.Flush(); err != nil {
		t.Fatalf("error while flushing writer")
	}

	return functions, names
}

func readLinesUntil(t *testing.T, reader *bufio.Reader, stop string, buffer *bytes.Buffer) error {
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			return fmt.Errorf("'%s' line not found", stop)
		}
		if err != nil {
			t.Fatalf("error while reading file: %v", err)
		}
		if line == stop {
			break
		}
		if buffer != nil {
			if _, err := buffer.WriteString(line); err != nil {
				t.Fatalf("error while buffering line")
			}
		}
	}
	return nil
}

func bufferExistingConversions(t *testing.T, fileName string) (bytes.Buffer, bytes.Buffer) {
	file, err := os.Open(fileName)
	if err != nil {
		t.Fatalf("couldn't open file %s", fileName)
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	functionsPrefix := "// AUTO-GENERATED FUNCTIONS START HERE\n"
	functionsSuffix := "// AUTO-GENERATED FUNCTIONS END HERE\n"
	if err := readLinesUntil(t, reader, functionsPrefix, nil); err != nil {
		t.Fatalf("error while parsing file: %v", err)
	}
	var functions bytes.Buffer
	if err := readLinesUntil(t, reader, functionsSuffix, &functions); err != nil {
		t.Fatalf("error while parsing file: %v", err)
	}

	functionNamesPrefix := "\terr := newer.Scheme.AddGeneratedConversionFuncs(\n"
	functionNamesSuffix := "\t)\n"
	if err := readLinesUntil(t, reader, functionNamesPrefix, nil); err != nil {
		t.Fatalf("error while parsing file: %v", err)
	}
	var names bytes.Buffer
	if err := readLinesUntil(t, reader, functionNamesSuffix, &names); err != nil {
		t.Fatalf("error while parsing file: %v", err)
	}

	return functions, names
}

func compareBuffers(t *testing.T, generatedFile string, existing, generated bytes.Buffer) bool {
	ok := true
	for {
		existingLine, existingErr := existing.ReadString('\n')
		generatedLine, generatedErr := generated.ReadString('\n')
		if existingErr == io.EOF && generatedErr == io.EOF {
			break
		}
		if existingErr != generatedErr {
			ok = false
			t.Errorf("reading errors: existing %v generated %v", existingErr, generatedErr)
			return ok
		}
		if existingErr != nil {
			ok = false
			t.Errorf("error while reading: %v", existingErr)
		}
		if existingLine != generatedLine {
			ok = false
			diff := fmt.Sprintf("first difference: expected %s, got %s", generatedLine, existingLine)
			t.Errorf("please update conversion functions; generated: %s; diff: %s", generatedFile, diff)
			return ok
		}
	}
	return ok
}

func TestNoManualChangesToGenerateConversions(t *testing.T) {
	versions := []string{"v1beta3", "v1"}

	for _, version := range versions {
		fileName := fmt.Sprintf("../../pkg/api/%s/conversion_generated.go", version)

		existingFunctions, existingNames := bufferExistingConversions(t, fileName)
		generatedFunctions, generatedNames := generateConversions(t, version)

		functionsTxt := fmt.Sprintf("%s.functions.txt", version)
		ioutil.WriteFile(functionsTxt, generatedFunctions.Bytes(), os.FileMode(0644))

		namesTxt := fmt.Sprintf("%s.names.txt", version)
		ioutil.WriteFile(namesTxt, generatedNames.Bytes(), os.FileMode(0644))

		if ok := compareBuffers(t, functionsTxt, existingFunctions, generatedFunctions); ok {
			os.Remove(functionsTxt)
		}
		if ok := compareBuffers(t, namesTxt, existingNames, generatedNames); ok {
			os.Remove(namesTxt)
		}
	}
}
