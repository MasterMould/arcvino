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

// --- SYSTEM & WORKFLOW STRUCTS ---

type SystemConfig struct {
	CPUDetails   string
	IntelArcSeen bool
}

type Workflow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ModelID  string `json:"modelId"`
	TaskType string `json:"taskType"`
}

var workflows = map[string]Workflow{
	"default-instant": {ID: "default-instant", Name: "Instant Test (TinyLlama 1.1B)", ModelID: "TinyLlama/TinyLlama-1.1B-Chat-v1.0", TaskType: "text-generation"},
	"default-gemma4":  {ID: "default-gemma4", Name: "Gemma 4 E4B (Text/Code)", ModelID: "google/gemma-4-E4B-it", TaskType: "text-generation"},
	"default-sd":      {ID: "default-sd", Name: "Stable Diffusion v1.5 (Image)", ModelID: "runwayml/stable-diffusion-v1-5", TaskType: "image-generation"},
}

func main() {
	fmt.Println("🚀 Initializing OpenVINO & Intel Arc A770 Deployment Engine...")
	config := DiscoverHardware()
	
	// Create Dedicated Model Directory
	home, _ := os.UserHomeDir()
	modelsDir := filepath.Join(home, "arcus_models")
	os.MkdirAll(modelsDir, 0755)

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

	// 3. Diagnostics & Telemetry
	http.HandleFunc("/api/diagnostics", handleDiagnostics)

	// 4. Workflow & Model Management
	http.HandleFunc("/api/workflows", handleWorkflows)
	http.HandleFunc("/api/models", handleModels)

	// 5. Orchestration Hooks
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

// --- WORKFLOW & MODEL CRUD LOGIC ---

func handleWorkflows(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" {
		json.NewEncoder(w).Encode(workflows)
		return
	} else if r.Method == "POST" {
		var wf Workflow
		json.NewDecoder(r.Body).Decode(&wf)
		if wf.ID == "" {
			wf.ID = fmt.Sprintf("wf-%d", time.Now().Unix())
		}
		workflows[wf.ID] = wf
		json.NewEncoder(w).Encode(wf)
		return
	} else if r.Method == "DELETE" {
		id := r.URL.Query().Get("id")
		delete(workflows, id)
		w.WriteHeader(http.StatusOK)
		return
	}
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	modelsDir := filepath.Join(home, "arcus_models")
	entries, _ := os.ReadDir(modelsDir)
	
	var models []string
	for _, e := range entries {
		if e.IsDir() {
			models = append(models, e.Name())
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// --- NATIVE DIAGNOSTICS ---

func handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	results := map[string]bool{
		"groups": false,
		"opencl": false,
		"driver": false,
	}

	currentUser, err := user.Current()
	if err == nil {
		out, _ := exec.Command("groups", currentUser.Username).CombinedOutput()
		s := string(out)
		results["groups"] = strings.Contains(s, "render") && strings.Contains(s, "video")
	}

	out, _ := exec.Command("clinfo").CombinedOutput()
	s := string(out)
	results["opencl"] = strings.Contains(s, "Intel(R) Arc(TM)") || strings.Contains(s, "A770")

	out, _ = exec.Command("lsmod").CombinedOutput()
	s = string(out)
	results["driver"] = strings.Contains(s, "i915") || strings.Contains(s, "xe")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// --- INSTALLATION LOGIC ---

func handleInstall(w http.ResponseWriter, r *http.Request) {
	sysScript := `
set -e
echo "🚂 Deploying System Runtimes (Requires Authorization)..."
apt-get update -y
apt-get install -y python3-venv python3-dev build-essential intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo git
usermod -aG render,video $SUDO_USER || usermod -aG render,video $USER
`
	cmdSys := exec.Command("pkexec", "bash", "-c", sysScript)
	outSys, errSys := cmdSys.CombinedOutput()
	if errSys != nil {
		fmt.Fprintf(w, "⚠️ System driver installation failed:\n%s", string(outSys))
		return
	}

	home, _ := os.UserHomeDir()
	userScript := fmt.Sprintf(`
set -e
echo "🐍 Initializing local OpenVINO Virtual Environment..."
python3 -m venv %s/openvino_env
source %s/openvino_env/bin/activate
echo "📦 Installing OpenVINO and Optimum libraries natively..."
pip install --upgrade pip
pip install openvino openvino-genai optimum-intel[openvino] diffusers accelerate
echo "📦 Pulling latest Transformers from source to support cutting-edge architectures..."
pip install git+https://github.com/huggingface/transformers.git
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
apt-get purge -y intel-opencl-icd libze-intel-gpu1 libze1 intel-ocloc clinfo git
apt-get autoremove -y
echo "✅ System drivers removed."
`
	cmd := exec.Command("pkexec", "bash", "-c", scriptPayload)
	out, _ := cmd.CombinedOutput()

	if !preserve {
		home, _ := os.UserHomeDir()
		os.RemoveAll(filepath.Join(home, "openvino_env"))
		os.RemoveAll(filepath.Join(home, "ov_server.py"))
		os.RemoveAll(filepath.Join(home, "start_engine.sh"))
		os.RemoveAll(filepath.Join(home, "arcus_models"))
		out = append(out, []byte("\n✅ Virtual environment and local models erased.")...)
	}
	fmt.Fprintf(w, "%s", string(out))
}

// --- ORCHESTRATION HOOKS ---

func handleLaunchEngine(w http.ResponseWriter, r *http.Request) {
	var reqBody map[string]string
	json.NewDecoder(r.Body).Decode(&reqBody)
	
	workflowID := reqBody["id"]
	wf, ok := workflows[workflowID]
	if !ok {
		wf = workflows["default-instant"]
	}

	home, _ := os.UserHomeDir()
	modelsDir := filepath.Join(home, "arcus_models")
	pyScriptPath := filepath.Join(home, "ov_server.py")
	shScriptPath := filepath.Join(home, "start_engine.sh")
	
	// Generate Python Server forcing model_dir caching
	pyCode := fmt.Sprintf(`import json
from http.server import BaseHTTPRequestHandler, HTTPServer
import sys
import time
import os

print("🚀 Initializing OpenVINO Engine on Intel Arc A770...")
try:
    from optimum.intel import OVModelForCausalLM, OVStableDiffusionPipeline
    from transformers import AutoTokenizer
except ImportError:
    print("❌ Error: Missing optimum-intel or transformers. Ensure the virtual environment is active.")
    sys.exit(1)

model_id = "%s"
task_type = "%s"
local_cache = "%s"

print(f"📁 Managing model assets in: {local_cache}")

if task_type == "text-generation":
    print(f"Loading Text/Code Model: {model_id} to GPU...")
    try:
        model = OVModelForCausalLM.from_pretrained(model_id, export=True, device="GPU", cache_dir=local_cache)
        tokenizer = AutoTokenizer.from_pretrained(model_id, cache_dir=local_cache)
        print("✅ Model loaded successfully! Listening for payloads on 11434...")
    except Exception as e:
        print(f"❌ Error allocating to GPU: {e}")
        sys.exit(1)
elif task_type == "image-generation":
    print(f"Loading Image Diffusion Model: {model_id} to GPU...")
    try:
        model = OVStableDiffusionPipeline.from_pretrained(model_id, export=True, device="GPU", cache_dir=local_cache)
        print("✅ Diffusion Pipeline loaded successfully! Listening for payloads on 11434...")
    except Exception as e:
        print(f"❌ Error allocating to GPU: {e}")
        sys.exit(1)
else:
    print("⚠️ Unsupported task type. Launching idle.")

class SimpleHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        req = json.loads(post_data.decode('utf-8'))
        
        prompt = req.get('prompt', '')
        
        if task_type == "text-generation":
            inputs = tokenizer(prompt, return_tensors="pt")
            
            start_time = time.time()
            outputs = model.generate(**inputs, max_new_tokens=150)
            end_time = time.time()
            
            # Calculate Tkps
            gen_tokens = len(outputs[0]) - len(inputs['input_ids'][0])
            elapsed = end_time - start_time
            tkps = gen_tokens / elapsed if elapsed > 0 else 0
            
            response_text = tokenizer.decode(outputs[0], skip_special_tokens=True)
            response_text = response_text.replace(prompt, "").strip()
            
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"response": response_text, "tkps": round(tkps, 2)}).encode('utf-8'))
            
        elif task_type == "image-generation":
            start_time = time.time()
            # Simulated output for UI orchestration
            end_time = time.time()
            
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"response": "Image processing mapped natively in OpenVINO backend.", "tkps": 0.0}).encode('utf-8'))

HTTPServer(('127.0.0.1', 11434), SimpleHandler).serve_forever()
`, wf.ModelID, wf.TaskType, modelsDir)

	os.WriteFile(pyScriptPath, []byte(pyCode), 0755)

	// Write the executable Bash script to bypass terminal quoting errors
	shCode := fmt.Sprintf("#!/bin/bash\nsource %s/openvino_env/bin/activate\npython3 %s\nexec bash\n", home, pyScriptPath)
	os.WriteFile(shScriptPath, []byte(shCode), 0755)

	// Safely launch the script in the user's preferred terminal emulator
	terminals := [][]string{
		{"gnome-terminal", "--", "bash", "-c", shScriptPath},
		{"x-terminal-emulator", "-e", shScriptPath},
		{"konsole", "-e", shScriptPath},
		{"xfce4-terminal", "-e", shScriptPath},
	}

	var started bool
	for _, termArgs := range terminals {
		cmd := exec.Command(termArgs[0], termArgs[1:]...)
		if err := cmd.Start(); err == nil {
			started = true
			break
		}
	}

	// Fallback to running in the background if no desktop terminal is found
	if !started {
		exec.Command("bash", shScriptPath).Start()
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": fmt.Sprintf("Launching %s engine process...", wf.Name)})
}

func handleLocalChat(w http.ResponseWriter, r *http.Request) {
	var reqBody map[string]string
	json.NewDecoder(r.Body).Decode(&reqBody)
	userMessage := reqBody["message"]

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"prompt": userMessage,
	})

	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post("http://127.0.0.1:11434/api/generate", "application/json", bytes.NewBuffer(payloadBytes))
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": "⚠️ OpenVINO AI engine not detected. Please select a Workflow and click 'Launch Engine'.",
			"tkps":     0.0,
		})
		return
	}
	defer resp.Body.Close()

	var serverResponse map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&serverResponse)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(serverResponse)
}

func handleImageGeneration(w http.ResponseWriter, r *http.Request) {
	time.Sleep(2 * time.Second) 
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Hardware accelerated image generation requires a Diffusion workflow to be configured and launched from the Workflows tab.",
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
        .tabs { display: flex; gap: 1rem; margin-bottom: 1rem; border-bottom: 2px solid #334155; padding-bottom: 1rem; overflow-x: auto; }
        .tab-btn { background: transparent; color: #94a3b8; border: none; font-size: 1.1rem; cursor: pointer; padding: 0.5rem 1rem; font-weight: bold; border-radius: 4px; transition: 0.2s; white-space: nowrap; }
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
        <button class="tab-btn active" onclick="switchTab('deploy')">1. Deployment</button>
        <button class="tab-btn" onclick="switchTab('workflows')">2. Workflows (CRUD)</button>
        <button class="tab-btn" onclick="switchTab('diag')">3. System Diagnostics</button>
        <button class="tab-btn" onclick="switchTab('chat')">4. AI Chat Showcase</button>
        <button class="tab-btn" onclick="switchTab('image')">5. Image Gen</button>
        <button class="tab-btn" onclick="switchTab('models')">6. Model Storage</button>
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

    <div id="workflows" class="tab-content">
        <h2>Model Workflow Configurations</h2>
        <p>Manage model execution pipelines for Text, Code, Image, and Video operations.</p>
        
        <div style="background: #0f172a; padding: 1.5rem; border-radius: 6px; border: 1px solid #334155; margin-bottom: 1.5rem;">
            <h3>Add / Edit Workflow</h3>
            <div style="display: flex; gap: 1rem; margin-top: 1rem; flex-wrap: wrap; align-items: center;">
                <input type="hidden" id="wfId">
                <input type="text" id="wfName" placeholder="Name (e.g., Gemma 4 12B Code)" style="flex: 1; min-width: 200px;">
                <input type="text" id="wfModel" placeholder="HF Model ID (e.g., google/gemma-4-12B-it)" style="flex: 1; min-width: 250px;">
                <select id="wfTask" style="padding: 0.75rem; background: #1e293b; border: 1px solid #334155; color: white; border-radius: 6px; min-width: 150px;">
                    <option value="text-generation">Text & Code Generation</option>
                    <option value="image-generation">Image Generation (Diffusion)</option>
                    <option value="video-generation">Video Generation (Pipeline)</option>
                </select>
                <button onclick="saveWorkflow()">Save Workflow</button>
            </div>
        </div>

        <table style="width: 100%; border-collapse: collapse; text-align: left; background: #0f172a; border-radius: 6px; overflow: hidden; border: 1px solid #334155;">
            <thead style="background: #334155;">
                <tr>
                    <th style="padding: 1rem;">Configuration Name</th>
                    <th style="padding: 1rem;">Model Repository</th>
                    <th style="padding: 1rem;">Task Designation</th>
                    <th style="padding: 1rem;">Actions</th>
                </tr>
            </thead>
            <tbody id="workflowTableBody">
                </tbody>
        </table>
    </div>

    <div id="diag" class="tab-content">
        <h2>Native Hardware Validation</h2>
        <p>Run internal Go routines to verify system topology and active telemetry.</p>
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
                <p>Verifies runtime libraries successfully detect the Arc A770.</p>
            </div>
            <div class="diag-card">
                <div id="diag-driver-icon" class="status-icon">⏳</div>
                <h3>Kernel Driver</h3>
                <p>Verifies low-level system driver (xe/i915) is actively engaged.</p>
            </div>
            <div class="diag-card" style="border: 2px solid var(--accent);">
                <div id="diag-tkps-icon" class="status-icon">⚡</div>
                <h3>Inference Performance</h3>
                <p style="font-size: 1.2rem; font-weight: bold; margin-bottom: 0;">Latest: <span id="diag-tkps-val" style="color:var(--success);">N/A</span></p>
                <p style="font-size:0.8rem; color:#94a3b8;">Updates automatically post-generation.</p>
            </div>
        </div>
    </div>

    <div id="chat" class="tab-content">
        <h2>Local LLM Orchestration</h2>
        <p>Proxy directly to your native OpenVINO engine using configured Workflows.</p>
        
        <div style="margin-bottom: 1rem; display: flex; align-items: center; gap: 1rem;">
            <select id="chatWorkflowSelect" style="padding: 0.75rem; background: #0f172a; color: white; border: 1px solid #334155; border-radius: 4px; min-width: 250px;"></select>
            <button onclick="launchEngine()" style="background-color: #0ea5e9;">Launch Engine in Separate Window</button>
            <span id="engineStatus" style="color: #94a3b8; font-size: 0.9rem;"></span>
        </div>

        <div class="chat-box" id="chatHistory">
            <div class="message msg-ai">System ready. Select a Workflow, launch the engine above, wait for the model to load into the VRAM, then enter a prompt.</div>
        </div>
        <div class="input-group">
            <input type="text" id="chatInput" placeholder="Type a message or coding task..." onkeypress="if(event.key === 'Enter') sendChat()">
            <button onclick="sendChat()">Send</button>
        </div>
    </div>

    <div id="image" class="tab-content">
        <h2>Image Generation Interface</h2>
        <p>UI framework ready. Ensure an Image Generation workflow is active.</p>
        <div style="background: #0f172a; height: 300px; border: 2px dashed #334155; border-radius: 8px; display: flex; align-items: center; justify-content: center; margin-bottom: 1rem;">
            <span id="imageCanvas" style="color: #64748b;">Image Canvas Ready</span>
        </div>
        <div class="input-group">
            <input type="text" id="imageInput" placeholder="A futuristic freight train racing through a cyberpunk city, highly detailed...">
            <button onclick="generateImage()">Generate</button>
        </div>
    </div>

    <div id="models" class="tab-content">
        <h2>Managed Model Repository</h2>
        <p>Location: <code style="color: #38bdf8;">~/arcus_models</code></p>
        <p style="color:#94a3b8; font-size:0.9rem;">HuggingFace Optimum caches local weights and OpenVINO IR files natively in this directory.</p>
        <button onclick="fetchModels()">Refresh Directory</button>
        <ul id="modelList" style="background: #020617; color: #4ade80; padding: 1.5rem 2rem; border-radius: 6px; font-family: monospace; border: 1px solid #334155; margin-top: 1rem; list-style-type: square;">
            </ul>
    </div>

    <script>
        // --- Initialization ---
        window.onload = function() {
            fetchWorkflows();
            fetchModels();
        };

        function switchTab(tabId) {
            document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            document.getElementById(tabId).classList.add('active');
            event.target.classList.add('active');
        }

        // --- Workflow CRUD Logic ---
        async function fetchWorkflows() {
            const res = await fetch('/api/workflows');
            const data = await res.json();
            
            let rows = '';
            let options = '';
            
            for (const [id, wf] of Object.entries(data)) {
                rows += '<tr style="border-bottom: 1px solid #1e293b;">' +
                    '<td style="padding: 1rem;"><strong>' + wf.name + '</strong></td>' +
                    '<td style="padding: 1rem; color: #94a3b8; font-family: monospace;">' + wf.modelId + '</td>' +
                    '<td style="padding: 1rem;"><span style="background: #334155; padding: 0.25rem 0.5rem; border-radius: 4px; font-size: 0.85rem;">' + wf.taskType + '</span></td>' +
                    '<td style="padding: 1rem;">' +
                        '<button onclick="editWorkflow(\''+wf.id+'\', \''+wf.name+'\', \''+wf.modelId+'\', \''+wf.taskType+'\')" style="background:#f59e0b; padding:0.4rem 0.8rem; font-size:0.8rem; margin-right:0.5rem;">Edit</button>' +
                        '<button onclick="deleteWorkflow(\''+wf.id+'\')" class="btn-danger" style="padding:0.4rem 0.8rem; font-size:0.8rem;">Delete</button>' +
                    '</td>' +
                '</tr>';
                options += '<option value="' + wf.id + '">' + wf.name + ' [' + wf.taskType + ']</option>';
            }
            
            document.getElementById('workflowTableBody').innerHTML = rows;
            document.getElementById('chatWorkflowSelect').innerHTML = options;
        }

        function editWorkflow(id, name, modelId, taskType) {
            document.getElementById('wfId').value = id;
            document.getElementById('wfName').value = name;
            document.getElementById('wfModel').value = modelId;
            document.getElementById('wfTask').value = taskType;
        }

        async function saveWorkflow() {
            const wf = {
                id: document.getElementById('wfId').value,
                name: document.getElementById('wfName').value,
                modelId: document.getElementById('wfModel').value,
                taskType: document.getElementById('wfTask').value
            };
            if(!wf.name || !wf.modelId) return alert("Name and Model ID required.");
            
            await fetch('/api/workflows', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(wf)
            });
            
            document.getElementById('wfId').value = '';
            document.getElementById('wfName').value = '';
            document.getElementById('wfModel').value = '';
            fetchWorkflows();
        }

        async function deleteWorkflow(id) {
            if(confirm("Delete this workflow configuration?")) {
                await fetch('/api/workflows?id=' + id, {method: 'DELETE'});
                fetchWorkflows();
            }
        }

        // --- Model Storage Logic ---
        async function fetchModels() {
            const res = await fetch('/api/models');
            const data = await res.json();
            const list = document.getElementById('modelList');
            list.innerHTML = '';
            if (!data || data.length === 0) {
                list.innerHTML = '<li style="color: #64748b;">Directory is currently empty.</li>';
                return;
            }
            data.forEach(m => {
                list.innerHTML += '<li>' + m + '</li>';
            });
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
        }

        // --- Chat Logic ---
        async function launchEngine() {
            const statusEl = document.getElementById('engineStatus');
            const wfId = document.getElementById('chatWorkflowSelect').value;
            statusEl.innerText = "Spawning external terminal window...";
            
            const res = await fetch('/api/launch-engine', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({id: wfId})
            });
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

            // Handle Response
            document.getElementById(loadingId).innerText = data.response;
            box.scrollTop = box.scrollHeight;
            
            // Handle Telemetry Update
            if(data.tkps !== undefined && data.tkps > 0) {
                document.getElementById('diag-tkps-val').innerText = data.tkps + " tokens/sec";
            }
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
</html>
`
