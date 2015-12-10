// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"cmd/internal/goobj"
	"cmd/internal/obj"
)

func objdumpcmd(fname string) *exec.Cmd {
	GOARCH := obj.Getgoarch()
	switch GOARCH {
	case "arm":
		return exec.Command(
			"arm-none-eabi-objdump",
			"-b", "binary",
			"-m", "arm",
			"-EL",
			"-D", fname)
	case "riscv":
		return exec.Command(
			"riscv64-unknown-elf-objdump",
			"-b", "binary",
			"-m", "riscv:rv64",
			"-EL",
			"-D", fname)
	default:
		log.Fatalf("unsupported architecture %s", GOARCH)
	}
	return nil
}

func disas1(sym *goobj.Sym, data []byte) {
	f, err := ioutil.TempFile("/tmp", "go_disas")
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()
	defer os.Remove(f.Name())

	_, err = f.Write(data)
	if err != nil {
		log.Println(err)
		return
	}

	objdumpout, err := objdumpcmd(f.Name()).Output()
	if err != nil {
		log.Println(err)
		return
	}
	objdumplines := strings.Split(string(objdumpout[:]), "\n")
	fmt.Printf("%s:\n", sym.Name)
	for _, line := range objdumplines[7:] {
		fmt.Println(line)
	}
}

func disas(file string, pkgpath string) {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	obj, err := goobj.Parse(f, pkgpath)
	if err != nil {
		log.Fatal(err)
	}

	for _, sym := range obj.Syms {
		data := make([]byte, sym.Data.Size)

		_, err = f.Seek(sym.Data.Offset, 0)
		if err != nil {
			log.Println(err)
			continue
		}

		_, err = f.Read(data)
		if err != nil {
			log.Println(err)
			continue
		}

		disas1(sym, data)
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("disas: ")

	// Ensure that we actually support this architecture.
	objdumpcmd("dummy")

	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: disas file.o\n")
		os.Exit(2)
	}

	disas(flag.Arg(0), "main")
}
