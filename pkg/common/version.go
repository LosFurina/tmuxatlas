package common

// NAME of the App
var NAME = "tmuxatlas"

// SUMMARY of the Version
var SUMMARY = "v0.2.1"

// BRANCH of the Version
var BRANCH = "dev"

// VERSION of Release
var VERSION = "0.2.1"

var COMMIT = "dirty"

// AppVersion --
var AppVersion AppVersionInfo

// AppVersionInfo --
type AppVersionInfo struct {
	Name    string
	Version string
	Branch  string
	Summary string
	Commit  string
}

func init() {
	AppVersion = AppVersionInfo{
		Name:    NAME,
		Version: VERSION,
		Branch:  BRANCH,
		Summary: SUMMARY,
		Commit:  COMMIT,
	}
}
