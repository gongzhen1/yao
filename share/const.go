package share

// VERSION Yao App Engine Version
const VERSION = "1.0.0"

// PRVERSION Yao App Engine PR Commit
const PRVERSION = "3a58b7aef3f5-2026-03-14T12:40:27+0000"

// CUI Version
const CUI = "1.0.0"

// PRCUI CUI PR Commit
const PRCUI = "df6d4b1ae052-2026-03-14T12:44:51+0000"

// BUILDOPTIONS Build options (e.g., "-s -w", "-s -w +upx")
const BUILDOPTIONS = ""

// BUILDIN If true, the application will be built into a single artifact
const BUILDIN = false

// BUILDNAME The name of the artifact
const BUILDNAME = "yao"

// MoapiHosts the master mirror
var MoapiHosts = []string{
	"master.moapi.ai",
	"master-moon.moapi.ai",
	"master-earth.moapi.ai",
	"master-mars.moapi.ai",
	"master-venus.moapi.ai",
	"master-mercury.moapi.ai",
	"master-jupiter.moapi.ai",
	"master-saturn.moapi.ai",
	"master-uranus.moapi.ai",
	"master-neptune.moapi.ai",
	"master-pluto.moapi.ai",
}
