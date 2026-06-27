package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type SystemConfig struct {
	CPUDetails   string
	SelectedGPU  string
	IntelArcSeen bool
}

func main() {
	fmt.Println("🚀 Initializing OpenVINO & Intel Arc A770 Deployment Engine...")
	
	config := DiscoverHardware()
	
	// Serve the web interface directly from memory
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := strings.Replace(htmlTemplate, "{{CPU}}", config.CPUDetails, 1)
		if config.IntelArcSeen {
			html = strings.Replace(html, "{{GPU_STATUS}}", "Intel Arc A770 (16GB VRAM) - Highly Recommended", 1)
		} else {
			html = strings.Replace(html, "{{GPU_STATUS}}", "Standard Integrated Graphics", 1)
		}
		fmt.Fprint(w, html)
	})

	// Unstoppable single-pass installation pipeline
	http.HandleFunc("/api/install", func(w http.ResponseWriter, r *http.Request) {
		output, err := ExecuteFreightTrainInstall()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Installation Error:\n%s\n%v", output, err)
			return
		}
		fmt.Fprintf(w, "%s", output)
	})

	// Pristine rollback endpoint
	http.HandleFunc("/api/uninstall", func(w http.ResponseWriter, r *http.Request) {
		preserve := r.URL.Query().Get("preserve") == "true"
		output, _ := ExecuteUninstall(preserve)
		fmt.Fprintf(w, "%s", output)
	})

	// ---------------------------------------------------------
	// FREIGHT TRAIN NETWORKING: Dynamically request an open port
	// ---------------------------------------------------------
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("⚠️ Fatal error creating network listener: %v", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	targetURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	
	log.Printf("🌐 User interface securely bound and ready at %s", targetURL)
	
	log.Println("🖥️  Launching web browser...")
	exec.Command("xdg-open", targetURL).Start()

	log.Fatal(http.Serve(listener, nil))
}

func DiscoverHardware() SystemConfig {
	var config SystemConfig
	config.CPUDetails = "Unknown Processor"
	config.IntelArcSeen = false

	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "model name") {
				config.CPUDetails = strings.TrimSpace(strings.Split(line, ":")[1])
				break
			}
		}
	}

	if matches, err := os.ReadDir("/sys/class/drm"); err == nil {
		for _, match := range matches {
			if strings.HasPrefix(match.Name(), "card") {
				devFile := fmt.Sprintf("/sys/class/drm/%s/device/device", match.Name())
				if id, err := os.ReadFile(devFile); err == nil {
					if strings.Contains(string(id), "0x56a0") {
						config.IntelArcSeen = true
					}
				}
			}
		}
	}

	return config
}

// ExecuteFreightTrainInstall executes everything in one single pkexec call
func ExecuteFreightTrainInstall() (string, error) {
	log.Println("⚡ Commencing single-pass installation...")

	targetUser := os.Getenv("USER")
	if targetUser == "" {
		targetUser = "root" // Fallback if environment is stripped
	}

	// Bundled installation into one bash payload for a single password prompt.
	// We drop the external Intel repo and use the native Ubuntu packages.
	scriptPayload := fmt.Sprintf(`
set -e
echo "🚂 Access granted. Initiating Freight Train Deployment..."

echo "🔧 [1/4] Cleaning up previous repository conflicts and locks..."
rm -f /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock
dpkg --configure -a || true
rm -f /etc/apt/sources.list.d/intel-gpu.list
rm -f /usr/share/keyrings/intel-graphics.gpg

echo "🔧 [2/4] Repairing any broken package states..."
apt-get update -y
apt-get --fix-broken install -y

echo "🔧 [3/4] Deploying Native Ubuntu 26.04 OpenVINO & Arc A770 Runtimes..."
# We explicitly use libze-intel-gpu1 here to avoid the conflict with intel-level-zero-gpu
apt-get install -y intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo

echo "👤 [4/4] Setting user permissions: Adding '%s' to render and video groups..."
usermod -aG render,video %s

echo "✅ Freight Train Installation Complete. The Intel Arc A770 is natively primed."
`, targetUser, targetUser)

	cmd := exec.Command("pkexec", "bash", "-c", scriptPayload)
	out, err := cmd.CombinedOutput()
	
	log.Printf("Installer Output:\n%s", string(out))
	
	return string(out), err
}

func ExecuteUninstall(preserveFiles bool) (string, error) {
	log.Println("🧹 Reverting changes clean...")
	
	scriptPayload := `
set -e
echo "🚂 Initiating Systematic Rollback..."
echo "🗑️ Purging Intel Compute Runtimes..."
apt-get purge -y intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo
apt-get autoremove -y
echo "✅ Software packages removed."
`
	cmd := exec.Command("pkexec", "bash", "-c", scriptPayload)
	out, err := cmd.CombinedOutput()

	if !preserveFiles {
		os.RemoveAll("/opt/openvino-installer/")
		out = append(out, []byte("\n✅ /opt/openvino-installer/ directory completely erased.")...)
	} else {
		out = append(out, []byte("\n💾 Asset retention enabled. Models preserved.")...)
	}
	
	return string(out), err
}

// Inlined UI asset layout featuring an embedded log console
const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>OpenVINO Installer Panel</title>
    <style>
        body { font-family: system-ui, sans-serif; background: #0f172a; color: #f8fafc; padding: 2rem; max-width: 900px; margin: 0 auto; }
        .card { background: #1e293b; padding: 1.5rem; border-radius: 8px; margin-bottom: 1rem; border: 1px solid #334155; }
        .btn { background: #2563eb; color: white; border: none; padding: 0.75rem 1.5rem; border-radius: 4px; cursor: pointer; font-weight: bold; }
        .btn-danger { background: #dc2626; }
        .status { color: #38bdf8; font-weight: bold; }
        .console { background: #000; color: #0f0; padding: 1rem; border-radius: 4px; font-family: monospace; height: 300px; overflow-y: auto; white-space: pre-wrap; margin-top: 1rem; display: none; }
    </style>
</head>
<body>
    <h1>OpenVINO Automated Deployer</h1>
    <div class="card">
        <h3>Detected Hardware Topology</h3>
        <p><strong>CPU:</strong> {{CPU}}</p>
        <p><strong>Target Acceleration Device:</strong> <span class="status">{{GPU_STATUS}}</span></p>
    </div>
    <div class="card">
        <h3>Actions</h3>
        <button class="btn" onclick="runInstall()">Launch Installation Pipeline</button>
        <hr style="border-color: #334155; margin: 1.5rem 0;">
        <label><input type="checkbox" id="preserve" checked> Preserve downloaded neural network weights on deletion</label><br><br>
        <button class="btn btn-danger" onclick="runUninstall()">Completely Uninstall</button>
        
        <div id="logOutput" class="console"></div>
    </div>
    <script>
        function showLog(message) {
            const logWindow = document.getElementById('logOutput');
            logWindow.style.display = 'block';
            logWindow.innerText = message;
        }

        async function runInstall() {
            showLog("🚂 Awaiting authorization... Please enter your password in the pop-up.\nRunning installation pipeline. Please wait...");
            try {
                const response = await fetch('/api/install');
                const text = await response.text();
                showLog(text);
            } catch (err) {
                showLog("⚠️ Network error communicating with backend.");
            }
        }

        async function runUninstall() {
            const preserve = document.getElementById('preserve').checked;
            showLog("🚂 Awaiting authorization... Please enter your password in the pop-up.\nRunning systematic rollback...");
            try {
                const response = await fetch('/api/uninstall?preserve=' + preserve);
                const text = await response.text();
                showLog(text);
            } catch (err) {
                showLog("⚠️ Network error communicating with backend.");
            }
        }
    </script>
</body>
</html>`
