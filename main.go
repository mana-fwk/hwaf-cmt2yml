package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func handle_err(err error) {
	if err != nil {
		panic(fmt.Errorf("hwaf-cmt2yml: %v", err))
	}
}

var g_profile_name = flag.String("profile", "atlasoff", "name of the profile translator to use")

func main() {
	fmt.Printf("::: hwaf-cmt2yml\n")

	flag.Parse()
	ok := false
	g_profile, ok = g_profiles[*g_profile_name]
	if !ok {
		profile_names := make([]string, 0, len(g_profiles))
		for k, _ := range g_profiles {
			profile_names = append(profile_names, k)
		}
		fmt.Fprintf(
			os.Stderr,
			"cmt2yml: invalid profile name (%s). valid ones are: %v\n",
			*g_profile_name,
			profile_names,
		)
		os.Exit(1)
	}

	dir := "."
	switch len(flag.Args()) {
	case 0:
		dir = "."
	case 1:
		dir = flag.Args()[0]
	default:
		panic(fmt.Errorf("cmt2yml takes at most 1 argument (got %d)", len(flag.Args())))
	}

	var err error
	//dir, err = filepath.Abs(dir)
	handle_err(err)

	fnames := []string{}
	fmt.Printf(">>> dir=%q\n", dir)
	if !path_exists(dir) {
		fmt.Printf("** no such file or directory [%s]\n", dir)
		os.Exit(1)
	}

	err = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		//fmt.Printf("::> [%s]...\n", path)
		if filepath.Base(path) != "requirements" {
			return nil
		} else {
			// check whether a non-automatically generated hscript.py or hscript.yml
			// already exist
			pkgdir := filepath.Dir(filepath.Dir(path))
			usr_file := false
			if path_exists(filepath.Join(pkgdir, "hscript.yml")) {
				usr_file = is_user_file(filepath.Join(pkgdir, "hscript.yml"))
				if usr_file {
					fmt.Printf("** discard [%s] (user-written hscript.yml)\n", pkgdir)
				}
			}
			if path_exists(filepath.Join(pkgdir, "hscript.py")) {
				usr_file = is_user_file(filepath.Join(pkgdir, "hscript.py"))
				if usr_file {
					fmt.Printf("** discard [%s] (user-written hscript.py)\n", pkgdir)
				}
			}
			if !usr_file {
				fnames = append(fnames, path)
				fmt.Printf("::> [%s]...\n", path)
			}

		}
		return err
	})
	handle_err(err)

	if len(fnames) < 1 {
		fmt.Printf(":: hwaf-cmt2yml: no requirements file under [%s]\n", dir)
		os.Exit(0)
	}

	type Response struct {
		req *ReqFile
		err error
	}

	// limit how many goroutines we have in flight
	// so we don't max out the number of open file descriptors
	throttle := make(chan struct{}, 100)
	ch := make(chan Response)
	for _, fname := range fnames {
		go func(fname string) {
			throttle <- struct{}{}
			reqfile, err := parse_file(fname)
			if err != nil {
				<-throttle
				ch <- Response{
					reqfile,
					fmt.Errorf("(parse) err w/ file [%s]: %v", fname, err),
				}
				return
			}
			err = render_script(reqfile)
			if err != nil {
				<-throttle
				ch <- Response{
					reqfile,
					fmt.Errorf("(render) err w/ file [%s]: %v", fname, err),
				}
				return
			}
			<-throttle
			ch <- Response{reqfile, nil}
		}(fname)
	}

	sum := 0
	allgood := true
loop:
	for {
		select {
		case resp := <-ch:
			sum += 1
			if resp.err != nil {
				fmt.Printf("**err: %v\n", resp.err)
				allgood = false
			}
			if sum == len(fnames) {
				close(ch)
				close(throttle)
				break loop
			}
		}
	}

	if !allgood {
		os.Exit(1)
	}
}

// EOF
