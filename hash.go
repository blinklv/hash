// hash.go
//
// Author: blinklv <blinklv@icloud.com>
// Create Time: 2018-01-17
// Maintainer: blinklv <blinklv@icloud.com>
// Last Change: 2018-01-23

// A simple command tool to calculate the HASH value of files. It supports
// some mainstream HASH algorithms, like MD5, FNV family and SHA family.
package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	h hash.Hash

	hashMap = map[string]hash.Hash{
		"md5":        md5.New(),
		"sha1":       sha1.New(),
		"sha224":     sha256.New224(),
		"sha256":     sha256.New(),
		"sha384":     sha512.New384(),
		"sha512":     sha512.New(),
		"sha512/224": sha512.New512_224(),
		"sha512/256": sha512.New512_256(),
		"fnv32":      fnv.New32(),
		"fnv32a":     fnv.New32a(),
		"fnv64":      fnv.New64(),
		"fnv64a":     fnv.New64a(),
		"fnv128":     fnv.New128(),
		"fnv128a":    fnv.New128a(),
	}
)

func main() {
	parseArg()
	return
}

// Parse the command arguments.
func parseArg() {
	if len(os.Args) == 1 {
		exit(1, "arguments are empty")
	}

	if h = hashMap[os.Args[1]]; h == nil {
		switch os.Args[1] {
		case "version":
			version()
		case "help":
			exit(0, "")
		default:
			exit(1, fmt.Sprintf("invalid option (%s)", os.Args[1]))
		}
	} else if len(os.Args) == 2 {
		// There must exist one file at least when the first argument
		// is the name of a hash algorithm.
		exit(1, "file arguments are empty")
	}

	return
}

// Print error, usage information and exit the process.
func exit(code int, msg string) {
	if code != 0 {
		fmt.Printf("error: %s!\n\n", msg)
	}
	usage()
	os.Exit(code)
}

// Print usage information for helping people to use this command correctly.
func usage() {
	msgs := []string{
		"usage: hash [algorithm|version|help] file [file...]\n",
		"\n",
		"       version   - print version information.\n",
		"       help      - print usage.\n",
		"       algorithm - the hash algorithm for computing the digest of files.\n",
		"                   Its values can be one in the following list:\n",
		"\n",
		"                   md5, sha1, sha224, sha256, sha384, sha512, sha512/224\n",
		"                   sha512/256, fnv32, fnv32a, fnv64, fnv64a, fnv128, fnv128a\n",
		"\n",
		"       file      - the objective file of the hash algorithm. If its type is directory,\n",
		"                   computing digest of all files in this directory recursively.\n",
	}

	for _, msg := range msgs {
		fmt.Printf(msg)
	}
}

const binary = "1.0.0"

// Print version information and exit the process.
func version() {
	fmt.Printf("%s v%s (built w/%s)\n", "hash", binary, runtime.Version())
	os.Exit(0)
}

type result struct {
	path string
	sum  []byte
	err  error
}

func hashAll(roots []string, done chan struct{}) (map[string][]byte, error) {
	results, errc := sum(roots, done)
	m := make(map[string][]byte)
	for r := range results {
		if r.err != nil {
			return nil, r.err
		}
		m[r.path] = r.sum
	}

	for err := range errc {
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// If we allocate a goroutine for each path immediately, it will cost resources heavy
// when there're so many big files. So we should limit the number of goroutines computing
// digest to reduce side effects for OS.
const numDigester = 16

func sum(roots []string, done <-chan struct{}) (<-chan result, <-chan error) {
	results := make(chan result)
	paths, errc := walk(roots, done)

	wg := &sync.WaitGroup{}
	wg.Add(numDigester)
	for i := 0; i < numDigester; i++ {
		go func() {
			digester(paths, results, done)
			wg.Done()
		}()
	}

	go func() {
		// We must ensure that no one writes to 'results' channel before closing it.
		wg.Wait()
		close(results)
	}()

	return results, errc
}

// Emits the paths for regular files in the tree.
func walk(roots []string, done <-chan struct{}) (<-chan string, <-chan error) {
	paths, errc := make(chan string), make(chan error, len(roots))
	go func() {
		wg := &sync.WaitGroup{}
		wg.Add(len(roots))

		for _, root := range roots {
			go func(root string) {
				// No select needed for this send, since errc is buffered.
				errc <- filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !info.Mode().IsRegular() {
						return nil
					}
					select {
					case paths <- path:
					case <-done:
						return errors.New("walk canceled")
					}
					return nil
				})
				wg.Done()
			}(root)
		}

		go func() {
			wg.Wait()

			// Close the paths and errc channel after Walk returns. If we don't do this operation,
			// 'digester' won't exit, then 'hashAll' won't exit too.
			close(paths)
			close(errc)
		}()
	}()
	return paths, errc
}

func digester(paths <-chan string, results chan<- result, done <-chan struct{}) {
	for path := range paths {
		data, err := ioutil.ReadFile(path)
		select {
		case results <- result{path, h.Sum(data), err}:
		case <-done:
			return
		}
	}
}
