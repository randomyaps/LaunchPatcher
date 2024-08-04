package main

import (
	"errors"
	"fmt"
	"github.com/logrusorgru/aurora/v3"
	"github.com/wii-tools/GoNUSD"
	"github.com/wii-tools/arclib"
	"github.com/wii-tools/powerpc"
	"github.com/wii-tools/wadlib"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
)

// baseDomain holds our needed base domain.
var baseDomain = "shop.8tx.icu"

// mainDol holds the main DOL - our content at index 1.
var mainDol []byte

// mainArc holds the main ARC - our content at index 2.
var mainArc *arclib.ARC

// rootCertificate holds the public certificate, in DER form, to be patched in.
var rootCertificate []byte

// filePresent returns whether the specified path is present on disk.
func filePresent(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist) == false
}

// createDir creates a directory at the given path if it is not already present.
func createDir(path string) {
	if !filePresent(path) {
		os.Mkdir(path, 0755)
	}
}

func main() {
	

	fmt.Println("===========================")
	fmt.Println("=       WSC-Patcher       =")
	fmt.Println("===========================")

	// Create directories we may need later.
	createDir("./output")
	createDir("./cache")

	var err error

	// Determine whether the Wii Shop Channel is cached.
	if !filePresent("./cache/original.wad") {
		log.Println("Downloading a copy of the original Wii Shop Channel, please wait...")
		var downloadedShop *wadlib.WAD
		downloadedShop, err = GoNUSD.Download(0x00010002_48414241, 21, true)
		check(err)

		// Cache this downloaded WAD to disk.
		var contents []byte
		contents, err = downloadedShop.GetWAD(wadlib.WADTypeCommon)
		check(err)

		os.WriteFile("./cache/original.wad", contents, 0755)
	}

	var originalWad *wadlib.WAD
	originalWad, err = wadlib.LoadWADFromFile("./cache/original.wad")
	check(err)

	// Determine whether a certificate authority was provided, or generated previously.
	if !filePresent("./output/root.cer") {
		fmt.Println(aurora.Green("Generating root certificates..."))
		rootCertificate = createCertificates()
	} else {
		rootCertificate, err = ioutil.ReadFile("./output/root.cer")
		check(err)
	}

	// Ensure the loaded certificate has a suitable length.
	// It must not be longer than 928 bytes.
	if len(rootCertificate) > 928 {
		fmt.Println("The passed root certificate exceeds the maximum length possible, 928 bytes.")
		fmt.Println("Please verify parameters passed for generation and reduce its size.")
		os.Exit(-1)
	}

	// Load main DOL
	mainDol, err = originalWad.GetContent(1)
	check(err)

	// Permit r/w access to MEM2_PROT via the TMD.
	// See docs/patch_overwrite_ios.md for more information!
	originalWad.TMD.AccessRightsFlags = 0x3
	// Apply all DOL patches
	fmt.Println(aurora.Green("Applying DOL patches..."))
	applyDefaultPatches()

	// Save main DOL
	err = originalWad.UpdateContent(1, mainDol)
	check(err)

	// Load main ARC
	arcData, err := originalWad.GetContent(2)
	check(err)
	mainArc, err = arclib.Load(arcData)
	check(err)

	// Generate filter list and certificate store
	fmt.Println(aurora.Green("Applying Opera patches..."))
	modifyAllowList()
	generateOperaCertStore()

	// Save main ARC
	updated, err := mainArc.Save()
	check(err)
	err = originalWad.UpdateContent(2, updated)
	check(err)

	// Generate a patched WAD with our changes
	output, err := originalWad.GetWAD(wadlib.WADTypeCommon)
	check(err)

	fmt.Println(aurora.Green("Done! Install ./output/patched.wad, sit back, and enjoy."))
	writeOut("patched.wad", output)
}

// applyDefaultPatches applies the default patches to our main DOL.
func applyDefaultPatches() {
	var err error

	mainDol, err = powerpc.ApplyPatchSet(OverwriteIOSPatch, mainDol)
	check(err)
	mainDol, err = powerpc.ApplyPatchSet(LoadCustomCA(), mainDol)
	check(err)
	mainDol, err = powerpc.ApplyPatchSet(PatchBaseDomain(), mainDol)
	check(err)
	mainDol, err = powerpc.ApplyPatchSet(NegateECTitle, mainDol)
	check(err)
	mainDol, err = powerpc.ApplyPatchSet(PatchECCfgPath, mainDol)
	check(err)
}

// check has an anxiety attack if things go awry.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// writeOut writes a file with the given name and contents to the output folder.
func writeOut(filename string, contents []byte) {
	os.WriteFile("./output/"+filename, contents, 0755)
}
