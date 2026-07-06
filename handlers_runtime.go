package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// handleLaunchEngine provisions the Python backend and maps the OpenVINO execution graph.
func handleLaunchEngine(w http.ResponseWriter, r *http.Request) {
	var wf LaunchOptions
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		http.Error(w, "Invalid structured parameters mapping context.", http.StatusBadRequest)
		return
	}

	home, _ := os.UserHomeDir()
	modelsDir := filepath.Join(home, "arcus_models")
	os.MkdirAll(modelsDir, 0755)

	pyScriptPath := filepath.Join(home, "ov_server.py")
	shScriptPath := filepath.Join(home, "start_engine.sh")

	// The isolated Python server template
	pyCode := fmt.Sprintf(`import sys, json, time, os
from http.server import HTTPServer, BaseHTTPRequestHandler
from transformers import AutoTokenizer, AutoProcessor
from optimum.intel.openvino import OVModelForCausalLM, OVModelForVisualCausalLM

model_id = "%s"
task_type = "%s"
storage_path = "%s"

print(f"📁 Managing model assets in: {storage_path}")

try:
    if task_type == "image-text-to-text":
        print(f"Loading Vision-Language Model: {model_id} to GPU...")
        model = OVModelForVisualCausalLM.from_pretrained(
            model_id,
            export=True,
            device="GPU",
            trust_remote_code=True,
            cache_dir=storage_path,
            load_in_low_bit="int8"
        )
        processor = AutoProcessor.from_pretrained(model_id, trust_remote_code=True, cache_dir=storage_path)
        # Extract underlying tokenizer to handle raw string prompts cleanly
        tokenizer = processor.tokenizer if hasattr(processor, "tokenizer") else processor
    else:
        print(f"Loading Text/Code Model: {model_id} to GPU...")
        model = OVModelForCausalLM.from_pretrained(
            model_id,
            export=True,
            device="GPU",
            trust_remote_code=True,
            cache_dir=storage_path,
            load_in_low_bit="int8"
        )
        tokenizer = AutoTokenizer.from_pretrained(model_id, trust_remote_code=True, cache_dir=storage_path)
        
    print("✅ Model loaded successfully! Listening for payloads on 11434...")
except Exception as e:
    print(f"❌ Error allocating to GPU: {e}")
    sys.exit(1)

class SimpleHandler(BaseHTTPRequestHandler):
    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'POST, GET, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')
        self.end_headers()

    def do_POST(self):
        self.do_OPTIONS()
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        data = json.loads(post_data.decode('utf-8'))
        prompt = data.get("prompt", "")
        req_task = data.get("task_type", "text-generation")
        
        if req_task in ["text-generation", "image-text-to-text"]:
            if hasattr(tokenizer, "chat_template") and tokenizer.chat_template is not None:
                messages = [{"role": "user", "content": prompt}]
                formatted_prompt = tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)
                inputs = tokenizer(formatted_prompt, return_tensors="pt").to(model.device)
            else:
                inputs = tokenizer(prompt, return_tensors="pt").to(model.device)
            
            start_time = time.time()
            outputs = model.generate(**inputs, max_new_tokens=250, temperature=0.7, top_p=0.9, do_sample=True)
            end_time = time.time()
            
            gen_tokens = len(outputs[0]) - len(inputs['input_ids'][0])
            elapsed = end_time - start_time
            tkps = gen_tokens / elapsed if elapsed > 0 else 0
            
            response_text = tokenizer.decode(outputs[0][len(inputs['input_ids'][0]):], skip_special_tokens=True).strip()
            
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"response": response_text, "tkps": round(tkps, 2)}).encode('utf-8'))

class ReuseHTTPServer(HTTPServer):
    allow_reuse_address = True

ReuseHTTPServer(('127.0.0.1', 11434), SimpleHandler).serve_forever()
`, wf.ModelID, wf.TaskType, modelsDir)

	if err := os.WriteFile(pyScriptPath, []byte(pyCode), 0644); err != nil {
		fmt.Fprintf(w, "❌ Error constructing Python compilation payload target: %v", err)
		return
	}

	// The execution wrapper
	shCode := fmt.Sprintf(`#!/bin/bash
echo "🧹 Scanning local process maps to clean overlapping engine configurations..."
pkill -f "%s" || true
sleep 1
source %s/openvino_env/bin/activate
python3 %s
exec bash
`, pyScriptPath, home, pyScriptPath)

	if err := os.WriteFile(shScriptPath, []byte(shCode), 0755); err != nil {
		fmt.Fprintf(w, "❌ Error constructing orchestration shell payload wrapper: %v", err)
		return
	}

	// Trigger execution context inside a dedicated interactive window environment
	cmd := exec.Command("gnome-terminal", "--", "bash", "-c", shScriptPath)
	if err := cmd.Start(); err != nil {
		// Fallback straight to background pipeline execution if target terminal window mechanism fails
		fallbackCmd := exec.Command("bash", "-c", shScriptPath)
		fallbackCmd.Start()
		fmt.Fprintln(w, "🚀 Engine process initialization mapped in headless operational space.")
		return
	}

	fmt.Fprintln(w, "🚀 Dedicated interface workspace instance attached over port 11434 successfully.")
}
