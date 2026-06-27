package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type SystemConfig struct {
	CPUDetails   string
	IntelArcSeen bool
}

func main() {
	fmt.Println("🚀 Initializing OpenVINO & Intel Arc A770 Deployment Engine...")
	config := DiscoverHardware()
	
	// 1. Core UI Endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := strings.Replace(htmlTemplate, "{{CPU}}", config.CPUDetails, 1)
		if config.IntelArcSeen {
			html = strings.Replace(html, "{{GPU_STATUS}}", "Intel Arc A770 (16GB VRAM) - Acceleration Ready", 1)
		} else {
			html = strings.Replace(html, "{{GPU_STATUS}}", "Standard Integrated Graphics", 1)
		}
		fmt.Fprint(w, html)
	})

	// 2. Installation Pipelines
	http.HandleFunc("/api/install", handleInstall)
	http.HandleFunc("/api/uninstall", handleUninstall)

	// 3. Native Go Diagnostics (No Bash Required)
	http.HandleFunc("/api/diagnostics", handleDiagnostics)

	// 4. Showcase API Hooks (Local Orchestration)
	http.HandleFunc("/api/launch-engine", handleLaunchEngine)
	http.HandleFunc("/api/chat", handleLocalChat)
	http.HandleFunc("/api/generate-image", handleImageGeneration)

	// ---------------------------------------------------------
	// FREIGHT TRAIN NETWORKING & BROWSER LAUNCH
	// ---------------------------------------------------------
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("⚠️ Fatal error creating network listener: %v", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	targetURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	
	log.Printf("🌐 Orchestration Dashboard securely bound at %s", targetURL)
	log.Println("🖥️  Launching web browser...")
	exec.Command("xdg-open", targetURL).Start()

	log.Fatal(http.Serve(listener, nil))
}

// --- HARDWARE DISCOVERY ---

func DiscoverHardware() SystemConfig {
	var config SystemConfig
	config.CPUDetails = "Unknown Processor"

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

// --- NATIVE DIAGNOSTICS ---

func handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	results := map[string]bool{
		"groups": false,
		"opencl": false,
		"driver": false,
	}

	// 1. Group Check natively via os/user
	currentUser, err := user.Current()
	if err == nil {
		out, _ := exec.Command("groups", currentUser.Username).CombinedOutput()
		s := string(out)
		results["groups"] = strings.Contains(s, "render") && strings.Contains(s, "video")
	}

	// 2. OpenCL Hardware Check
	out, _ := exec.Command("clinfo").CombinedOutput()
	s := string(out)
	results["opencl"] = strings.Contains(s, "Intel(R) Arc(TM)") || strings.Contains(s, "A770")

	// 3. Kernel Driver Check
	out, _ = exec.Command("lsmod").CombinedOutput()
	s = string(out)
	results["driver"] = strings.Contains(s, "i915") || strings.Contains(s, "xe")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// --- INSTALLATION LOGIC ---

func handleInstall(w http.ResponseWriter, r *http.Request) {
	// Step 1: System-level driver deployment (Requires Authorization)
	sysScript := `
set -e
echo "🚂 Deploying System Runtimes (Requires Authorization)..."
apt-get update -y
apt-get install -y python3-venv python3-dev build-essential intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo
usermod -aG render,video $SUDO_USER || usermod -aG render,video $USER
`
	cmdSys := exec.Command("pkexec", "bash", "-c", sysScript)
	outSys, errSys := cmdSys.CombinedOutput()
	if errSys != nil {
		fmt.Fprintf(w, "⚠️ System driver installation failed:\n%s", string(outSys))
		return
	}

	// Step 2: Unprivileged Python Virtual Environment (Standard User Execution)
	home, _ := os.UserHomeDir()
	userScript := fmt.Sprintf(`
set -e
echo "🐍 Initializing local OpenVINO Virtual Environment..."
python3 -m venv %s/openvino_env
source %s/openvino_env/bin/activate
echo "📦 Installing OpenVINO and Optimum libraries natively..."
pip install --upgrade pip
pip install openvino openvino-genai optimum-intel[openvino] transformers
echo "✅ OpenVINO Deployment Complete."
`, home, home)

	cmdUser := exec.Command("bash", "-c", userScript)
	outUser, _ := cmdUser.CombinedOutput()

	fmt.Fprintf(w, "%s\n%s", string(outSys), string(outUser))
}

func handleUninstall(w http.ResponseWriter, r *http.Request) {
	preserve := r.URL.Query().Get("preserve") == "true"
	scriptPayload := `
set -e
echo "🚂 Initiating Rollback..."
apt-get purge -y intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo
apt-get autoremove -y
echo "✅ System drivers removed."
`
	cmd := exec.Command("pkexec", "bash", "-c", scriptPayload)
	out, _ := cmd.CombinedOutput()

	if !preserve {
		home, _ := os.UserHomeDir()
		os.RemoveAll(filepath.Join(home, "openvino_env"))
		os.RemoveAll(filepath.Join(home, "ov_server.py"))
		out = append(out, []byte("\n✅ Virtual environment erased.")...)
	}
	fmt.Fprintf(w, "%s", string(out))
}

// --- SHOWCASE API HOOKS ---

func handleLaunchEngine(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	pyScriptPath := filepath.Join(home, "ov_server.py")
	
	pyCode := `import json
from http.server import BaseHTTPRequestHandler, HTTPServer
import sys

print("🚀 Initializing OpenVINO Engine on Intel Arc A770...")
try:
    from optimum.intel import OVModelForCausalLM
    from transformers import AutoTokenizer
except ImportError:
    print("❌ Error: Missing optimum-intel. Ensure the virtual environment is active.")
    sys.exit(1)

model_id = "TinyLlama/TinyLlama-1.1B-Chat-v1.0"
print(f"Loading {model_id} to GPU...")
try:
    model = OVModelForCausalLM.from_pretrained(model_id, export=True, device="GPU")
    tokenizer = AutoTokenizer.from_pretrained(model_id)
    print("✅ Model loaded successfully! Listening for chat payloads on 11434...")
except Exception as e:
    print(f"❌ Error allocating to GPU: {e}")
    sys.exit(1)

class SimpleHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        req = json.loads(post_data.decode('utf-8'))
        
        prompt = req.get('prompt', '')
        inputs = tokenizer(prompt, return_tensors="pt")
        outputs = model.generate(**inputs, max_new_tokens=150)
        
        response_text = tokenizer.decode(outputs[0], skip_special_tokens=True)
        # Clean up the prompt overlap for the UI
        response_text = response_text.replace(prompt, "").strip()
        
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps({"response": response_text}).encode('utf-8'))

HTTPServer(('127.0.0.1', 11434), SimpleHandler).serve_forever()
`
	os.WriteFile(pyScriptPath, []byte(pyCode), 0755)

	// Launch in a separate Linux terminal window
	launchCmd := fmt.Sprintf(`x-terminal-emulator -e "bash -c 'source %s/openvino_env/bin/activate && python3 %s; exec bash'" || gnome-terminal -- bash -c "source %s/openvino_env/bin/activate && python3 %s; exec bash"`, home, pyScriptPath, home, pyScriptPath)
	
	exec.Command("bash", "-c", launchCmd).Start()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "OpenVINO engine launching in a separate window..."})
}

func handleLocalChat(w http.ResponseWriter, r *http.Request) {
	var reqBody map[string]string
	json.NewDecoder(r.Body).Decode(&reqBody)
	userMessage := reqBody["message"]

	// Format payload for the local OpenVINO python server
	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"prompt": userMessage,
	})

	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post("http://127.0.0.1:11434/api/generate", "application/json", bytes.NewBuffer(payloadBytes))
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"response": "⚠️ OpenVINO AI engine not detected. Please click 'Launch Engine in Separate Window' first.",
		})
		return
	}
	defer resp.Body.Close()

	var serverResponse map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&serverResponse)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"response": fmt.Sprintf("%v", serverResponse["response"]),
	})
}

func handleImageGeneration(w http.ResponseWriter, r *http.Request) {
	time.Sleep(2 * time.Second) 
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Hardware accelerated image generation requires a backend server (e.g. sdcpp or AUTOMATIC1111) to be orchestrated on this machine. Your OpenCL drivers are installed and ready for these frameworks.",
	})
}

// --- EMBEDDED FRONTEND ---

const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Arcus Control Center</title>
    <style>
        :root { --bg: #0f172a; --panel: #1e293b; --accent: #3b82f6; --text: #f8fafc; --success: #22c55e; --danger: #ef4444; }
        body { font-family: system-ui, sans-serif; background: var(--bg); color: var(--text); padding: 2rem; max-width: 1000px; margin: 0 auto; }
        
        .header-panel { background: var(--panel); padding: 1.5rem; border-radius: 8px; border: 1px solid #334155; margin-bottom: 2rem; }
        .gpu-status { color: #38bdf8; font-weight: bold; text-shadow: 0 0 10px rgba(56, 189, 248, 0.4); }
        
        /* Tab Navigation */
        .tabs { display: flex; gap: 1rem; margin-bottom: 1rem; border-bottom: 2px solid #334155; padding-bottom: 1rem; }
        .tab-btn { background: transparent; color: #94a3b8; border: none; font-size: 1.1rem; cursor: pointer; padding: 0.5rem 1rem; font-weight: bold; border-radius: 4px; transition: 0.2s; }
        .tab-btn.active { background: var(--panel); color: var(--text); border-bottom: 3px solid var(--accent); }
        .tab-btn:hover:not(.active) { color: white; background: #334155; }
        
        .tab-content { display: none; background: var(--panel); padding: 2rem; border-radius: 8px; border: 1px solid #334155; }
        .tab-content.active { display: block; }

        /* Buttons & Inputs */
        button { background: var(--accent); color: white; border: none; padding: 0.75rem 1.5rem; border-radius: 6px; cursor: pointer; font-weight: bold; font-size: 1rem; transition: 0.2s; }
        button:hover { filter: brightness(1.2); }
        .btn-danger { background: var(--danger); }
        
        /* Console Logs */
        .console { background: #020617; color: #4ade80; padding: 1rem; border-radius: 6px; font-family: monospace; height: 300px; overflow-y: auto; white-space: pre-wrap; margin-top: 1rem; border: 1px solid #334155; }

        /* Diagnostics Grid */
        .diag-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 1rem; margin-top: 1.5rem; }
        .diag-card { background: #0f172a; padding: 1.5rem; border-radius: 8px; border: 1px solid #334155; display: flex; flex-direction: column; align-items: center; text-align: center; }
        .status-icon { font-size: 3rem; margin-bottom: 1rem; }
        .pass { color: var(--success); }
        .fail { color: var(--danger); }

        /* AI Chat UI */
        .chat-box { height: 300px; background: #0f172a; border-radius: 6px; padding: 1rem; overflow-y: auto; display: flex; flex-direction: column; gap: 1rem; border: 1px solid #334155; margin-bottom: 1rem; }
        .message { padding: 0.75rem 1rem; border-radius: 8px; max-width: 80%; }
        .msg-user { background: var(--accent); align-self: flex-end; }
        .msg-ai { background: #334155; align-self: flex-start; }
        .input-group { display: flex; gap: 0.5rem; }
        input[type="text"] { flex: 1; padding: 0.75rem; background: #0f172a; border: 1px solid #334155; color: white; border-radius: 6px; font-size: 1rem; }
    </style>
</head>
<body>

    <div class="header-panel">
        <h1 style="margin-top:0;">Arcus Control Center</h1>
        <p><strong>CPU:</strong> {{CPU}}</p>
        <p><strong>Target Device:</strong> <span class="gpu-status">{{GPU_STATUS}}</span></p>
    </div>

    <div class="tabs">
        <button class="tab-btn active" onclick="switchTab('deploy')">1. Deployment Pipeline</button>
        <button class="tab-btn" onclick="switchTab('diag')">2. System Diagnostics</button>
        <button class="tab-btn" onclick="switchTab('chat')">3. AI Chat Showcase</button>
        <button class="tab-btn" onclick="switchTab('image')">4. Image Gen</button>
    </div>

    <div id="deploy" class="tab-content active">
        <h2>OpenVINO Management</h2>
        <p>Deploy native Ubuntu Intel Arc compute runtimes and the OpenVINO Python Environment.</p>
        <div style="display:flex; gap:1rem; align-items:center;">
            <button onclick="runInstall()">Launch Installation Pipeline</button>
            <button class="btn-danger" onclick="runUninstall()">Completely Uninstall</button>
            <label style="margin-left:auto;"><input type="checkbox" id="preserve" checked> Preserve Models</label>
        </div>
        <div id="deployLog" class="console" style="display:none;"></div>
    </div>

    <div id="diag" class="tab-content">
        <h2>Native Hardware Validation</h2>
        <p>Run internal Go routines to verify system topology without dropping to bash.</p>
        <button onclick="runDiagnostics()">Execute Validation Scan</button>
        
        <div class="diag-grid" id="diagResults" style="display:none;">
            <div class="diag-card">
                <div id="diag-group-icon" class="status-icon">⏳</div>
                <h3>System Groups</h3>
                <p>Verifies your user account is in the 'render' and 'video' groups.</p>
            </div>
            <div class="diag-card">
                <div id="diag-opencl-icon" class="status-icon">⏳</div>
                <h3>OpenCL API</h3>
                <p>Verifies the runtime libraries successfully detect the Arc A770.</p>
            </div>
            <div class="diag-card">
                <div id="diag-driver-icon" class="status-icon">⏳</div>
                <h3>Kernel Driver</h3>
                <p>Verifies the low-level system driver (xe/i915) is actively engaged.</p>
            </div>
        </div>
    </div>

    <div id="chat" class="tab-content">
        <h2>Local LLM Orchestration</h2>
        <p>This interface proxies directly to your custom OpenVINO Python runtime.</p>
        
        <button onclick="launchEngine()" style="margin-bottom: 1rem; background-color: #0ea5e9;">Launch Engine in Separate Window</button>
        <span id="engineStatus" style="margin-left: 1rem; color: #94a3b8; font-size: 0.9rem;"></span>

        <div class="chat-box" id="chatHistory">
            <div class="message msg-ai">System ready. Launch the engine above, wait for the model to load into the GPU, then enter a prompt.</div>
        </div>
        <div class="input-group">
            <input type="text" id="chatInput" placeholder="Type a message..." onkeypress="if(event.key === 'Enter') sendChat()">
            <button onclick="sendChat()">Send</button>
        </div>
    </div>

    <div id="image" class="tab-content">
        <h2>Image Generation Interface</h2>
        <p>UI framework ready. Awaiting a local Stable Diffusion backend for processing.</p>
        <div style="background: #0f172a; height: 300px; border: 2px dashed #334155; border-radius: 8px; display: flex; align-items: center; justify-content: center; margin-bottom: 1rem;">
            <span id="imageCanvas" style="color: #64748b;">Image Canvas Ready</span>
        </div>
        <div class="input-group">
            <input type="text" id="imageInput" placeholder="A futuristic freight train racing through a cyberpunk city, highly detailed...">
            <button onclick="generateImage()">Generate</button>
        </div>
    </div>

    <script>
        function switchTab(tabId) {
            document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            document.getElementById(tabId).classList.add('active');
            event.target.classList.add('active');
        }

        // --- Deployment Logic ---
        function showLog(msg) {
            const el = document.getElementById('deployLog');
            el.style.display = 'block';
            el.innerText += msg + '\n';
            el.scrollTop = el.scrollHeight;
        }

        async function runInstall() {
            document.getElementById('deployLog').innerText = "";
            showLog("🚂 Awaiting authorization for core drivers...");
            const res = await fetch('/api/install');
            showLog(await res.text());
        }

        async function runUninstall() {
            document.getElementById('deployLog').innerText = "";
            const p = document.getElementById('preserve').checked;
            showLog("🚂 Awaiting authorization to rollback...");
            const res = await fetch('/api/uninstall?preserve=' + p);
            showLog(await res.text());
        }

        // --- Diagnostics Logic ---
        async function runDiagnostics() {
            document.getElementById('diagResults').style.display = 'grid';
            
            const ids = ['group', 'opencl', 'driver'];
            ids.forEach(id => {
                const el = document.getElementById('diag-' + id + '-icon');
                el.innerText = '⏳';
                el.className = 'status-icon';
            });

            const res = await fetch('/api/diagnostics');
            const data = await res.json();

            function setStatus(id, passed) {
                const el = document.getElementById('diag-' + id + '-icon');
                el.innerText = passed ? '✅' : '❌';
                el.className = 'status-icon ' + (passed ? 'pass' : 'fail');
            }

            setStatus('group', data.groups);
            setStatus('opencl', data.opencl);
            setStatus('driver', data.driver);

            if(!data.groups) alert("⚠️ Warning: You must log out and log back in for Group permissions to apply before AI models can use the GPU.");
        }

        // --- Chat Logic ---
        async function launchEngine() {
            const statusEl = document.getElementById('engineStatus');
            statusEl.innerText = "Spawning external terminal window...";
            const res = await fetch('/api/launch-engine');
            const data = await res.json();
            statusEl.innerText = data.status;
        }

        async function sendChat() {
            const input = document.getElementById('chatInput');
            const msg = input.value.trim();
            if(!msg) return;

            const box = document.getElementById('chatHistory');
            box.innerHTML += '<div class="message msg-user">' + msg + '</div>';
            input.value = '';
            box.scrollTop = box.scrollHeight;

            const loadingId = 'load-' + Date.now();
            box.innerHTML += '<div id="'+loadingId+'" class="message msg-ai">...</div>';
            box.scrollTop = box.scrollHeight;

            const res = await fetch('/api/chat', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({message: msg})
            });
            const data = await res.json();

            document.getElementById(loadingId).innerText = data.response;
            box.scrollTop = box.scrollHeight;
        }

        // --- Image Gen Logic ---
        async function generateImage() {
            const input = document.getElementById('imageInput').value;
            if(!input) return;
            const canvas = document.getElementById('imageCanvas');
            canvas.innerText = "Processing via backend...";
            
            const res = await fetch('/api/generate-image');
            const data = await res.json();
            
            canvas.innerText = data.status;
            canvas.style.color = "#3b82f6";
        }
    </script>
</body>
</html>`
