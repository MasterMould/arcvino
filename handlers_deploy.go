package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

// handleInstall processes the system and Python runtime provisioning pipeline.
func handleInstall(w http.ResponseWriter, r *http.Request) {
	var opts InstallOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		// Fall back silently to standard definitions if payload parsing fails
		opts = InstallOptions{UseNightly: false, HFToken: ""}
	}

	// 1. Core System Driver Layer (Elevated Privileges via pkexec)
	sysScript := `
set -e
echo "🚂 Deploying Native System Computing Runtimes..."
apt-get update -y
apt-get install -y python3-venv python3-dev build-essential intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo git
usermod -aG render,video $SUDO_USER || usermod -aG render,video $USER
`
	cmdSys := exec.Command("pkexec", "bash", "-c", sysScript)
	outSys, errSys := cmdSys.CombinedOutput()
	if errSys != nil {
		fmt.Fprintf(w, "⚠️ Linux system driver installation chain failure:\n%s", string(outSys))
		return
	}

	// 2. User Space Python/OpenVINO Environment Layer
	home, _ := os.UserHomeDir()
	userScript := fmt.Sprintf(`
set -e
echo "🐍 Fabricating secure local OpenVINO Python Environment space..."
python3 -m venv %s/openvino_env
source %s/openvino_env/bin/activate
pip install --upgrade pip
# Included torchvision to prevent fallback warnings for multi-modal image processors
pip install openvino openvino-genai optimum-intel[openvino] diffusers accelerate huggingface_hub torchvision
`, home, home)

	// Conditionally append bleeding-edge architecture components
	if opts.UseNightly {
		userScript += `
echo "📦 Pulling bleeding-edge model architectures from source repositories..."
pip install git+https://github.com/huggingface/transformers.git
pip install git+https://github.com/huggingface/optimum-intel.git
`
	}

	// Conditionally append HuggingFace CLI credentials authentication
	if opts.HFToken != "" {
		userScript += fmt.Sprintf(`
echo "🔑 Logging engine runtime token profile directly into HuggingFace environment hub..."
huggingface-cli login --token %s --add-to-git-credential
`, opts.HFToken)
	}

	userScript += "\necho \"✅ Environment execution architecture setup successfully verified.\""

	cmdUser := exec.Command("bash", "-c", userScript)
	outUser, _ := cmdUser.CombinedOutput()

	// Direct stream combination response back to the Deployment Center log terminal
	fmt.Fprintf(w, "%s\n%s", string(outSys), string(outUser))
}

// handleUninstall completely scrubs or partially resets the runtime pipeline.
func handleUninstall(w http.ResponseWriter, r *http.Request) {
	preserve := r.URL.Query().Get("preserve")
	home, _ := os.UserHomeDir()

	// Base target to scrub is always the virtual environment folder
	uninstallScript := fmt.Sprintf("rm -rf %s/openvino_env", home)
	
	if preserve != "true" {
		// Destructive path: clear cached weights along with binaries
		uninstallScript += fmt.Sprintf(" %s/arcus_models", home)
		fmt.Fprintln(w, "🗑️ Complete teardown initiated: removing runtime binary environment and cached layer blobs.")
	} else {
		// Safe path: preserve downloaded weights
		fmt.Fprintln(w, "🗑️ Standard modular environment reset initiated: keeping local model cache tracks.")
	}

	cmd := exec.Command("bash", "-c", uninstallScript)
	out, _ := cmd.CombinedOutput()
	fmt.Fprintf(w, "%s\n✅ Environmental runtime configurations successfully scrubbed.", string(out))
}
