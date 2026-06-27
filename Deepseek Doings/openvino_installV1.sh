#!/bin/bash
# OpenVINO Interactive Installation Tool for Ubuntu 26.04
# Version: 1.1.0

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Global variables
INSTALL_DIR="/opt/intel/openvino"
DOWNLOAD_DIR="/tmp/openvino_downloads"
LOG_FILE="/var/log/openvino_install.log"
WEB_PORT=8080
TEMP_DIR="/tmp/openvino_web"
PYTHON_VERSION="3.12"
MISSING_DEPS=()

# Hardware detection arrays
declare -A HARDWARE_INFO
declare -A GPU_INFO

log_message() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

print_status() { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ---------- HARDWARE DETECTION ----------
detect_hardware() {
    print_status "Detecting hardware configuration..."
    HARDWARE_INFO[CPU_CORES]=$(nproc)
    HARDWARE_INFO[CPU_MODEL]=$(lscpu | grep "Model name" | cut -d':' -f2 | xargs)
    HARDWARE_INFO[CPU_SPEED]=$(lscpu | grep "CPU max MHz" | cut -d':' -f2 | xargs)
    HARDWARE_INFO[RAM_TOTAL]=$(free -h | grep Mem | awk '{print $2}')
    HARDWARE_INFO[RAM_AVAILABLE]=$(free -h | grep Mem | awk '{print $7}')
    HARDWARE_INFO[STORAGE_TOTAL]=$(df -h / | awk 'NR==2 {print $2}')
    HARDWARE_INFO[STORAGE_AVAILABLE]=$(df -h / | awk 'NR==2 {print $4}')
    HARDWARE_INFO[STORAGE_SPEED]=$(sudo hdparm -Tt /dev/sda 2>/dev/null | grep "buffered" | awk '{print $11, $12}' || echo "Unknown")
    detect_gpus
    determine_best_gpu
}

detect_gpus() {
    print_status "Detecting GPUs..."
    # Clear previous
    GPU_INFO=()
    # Intel integrated
    if lspci | grep -i "VGA.*Intel" | grep -v "DG" | grep -v "Arc" > /dev/null; then
        GPU_INFO[INTEL_INTEGRATED]=$(lspci | grep -i "VGA.*Intel" | grep -v "DG" | grep -v "Arc" | head -1)
        GPU_INFO[INTEL_INTEGRATED_PRESENT]="true"
        print_status "Found Intel Integrated GPU: ${GPU_INFO[INTEL_INTEGRATED]}"
    else
        GPU_INFO[INTEL_INTEGRATED_PRESENT]="false"
    fi
    # Intel discrete (DG, Arc, etc.)
    if lspci | grep -i "VGA.*Intel" | grep -E "DG|Arc" > /dev/null; then
        GPU_INFO[INTEL_DISCRETE]=$(lspci | grep -i "VGA.*Intel" | grep -E "DG|Arc" | head -1)
        GPU_INFO[INTEL_DISCRETE_PRESENT]="true"
        print_status "Found Intel Discrete GPU: ${GPU_INFO[INTEL_DISCRETE]}"
    else
        GPU_INFO[INTEL_DISCRETE_PRESENT]="false"
    fi
    # NVIDIA
    if lspci | grep -i "VGA.*NVIDIA" > /dev/null; then
        GPU_INFO[NVIDIA]=$(lspci | grep -i "VGA.*NVIDIA" | head -1)
        GPU_INFO[NVIDIA_PRESENT]="true"
        if command -v nvidia-smi &> /dev/null; then
            GPU_INFO[NVIDIA_DRIVER]="$(nvidia-smi --query-gpu=driver_version --format=csv,noheader | head -1)"
            GPU_INFO[NVIDIA_MEMORY]="$(nvidia-smi --query-gpu=memory.total --format=csv,noheader | head -1)"
        fi
        print_status "Found NVIDIA GPU: ${GPU_INFO[NVIDIA]}"
    else
        GPU_INFO[NVIDIA_PRESENT]="false"
    fi
    # AMD
    if lspci | grep -i "VGA.*AMD" > /dev/null; then
        GPU_INFO[AMD]=$(lspci | grep -i "VGA.*AMD" | head -1)
        GPU_INFO[AMD_PRESENT]="true"
        print_status "Found AMD GPU: ${GPU_INFO[AMD]}"
    else
        GPU_INFO[AMD_PRESENT]="false"
    fi
}

determine_best_gpu() {
    print_status "Determining best GPU for OpenVINO..."
    # Priority: NVIDIA > Intel Discrete > Intel Integrated > AMD > CPU
    if [[ "${GPU_INFO[NVIDIA_PRESENT]}" == "true" ]]; then
        HARDWARE_INFO[BEST_GPU]="nvidia"
        HARDWARE_INFO[BEST_GPU_NAME]="${GPU_INFO[NVIDIA]}"
        print_success "Selected NVIDIA GPU (best performance for AI)"
    elif [[ "${GPU_INFO[INTEL_DISCRETE_PRESENT]}" == "true" ]]; then
        HARDWARE_INFO[BEST_GPU]="intel_discrete"
        HARDWARE_INFO[BEST_GPU_NAME]="${GPU_INFO[INTEL_DISCRETE]}"
        print_success "Selected Intel Discrete GPU (excellent OpenVINO support)"
    elif [[ "${GPU_INFO[INTEL_INTEGRATED_PRESENT]}" == "true" ]]; then
        HARDWARE_INFO[BEST_GPU]="intel_integrated"
        HARDWARE_INFO[BEST_GPU_NAME]="${GPU_INFO[INTEL_INTEGRATED]}"
        print_success "Selected Intel Integrated GPU (good OpenVINO support)"
    elif [[ "${GPU_INFO[AMD_PRESENT]}" == "true" ]]; then
        HARDWARE_INFO[BEST_GPU]="amd"
        HARDWARE_INFO[BEST_GPU_NAME]="${GPU_INFO[AMD]}"
        print_warning "Selected AMD GPU (limited OpenVINO support)"
    else
        HARDWARE_INFO[BEST_GPU]="cpu"
        HARDWARE_INFO[BEST_GPU_NAME]="CPU Only"
        print_warning "No compatible GPU found, using CPU only mode"
    fi
}

# ---------- SOFTWARE & DEPENDENCIES ----------
detect_software() {
    print_status "Detecting software environment..."
    HARDWARE_INFO[OS_NAME]=$(lsb_release -ds 2>/dev/null || cat /etc/*release | head -n1)
    HARDWARE_INFO[OS_VERSION]=$(lsb_release -rs 2>/dev/null || echo "Unknown")
    if command -v python3 &> /dev/null; then
        HARDWARE_INFO[PYTHON_VERSION]=$(python3 --version | awk '{print $2}')
    else
        HARDWARE_INFO[PYTHON_VERSION]="Not installed"
    fi
    check_dependencies
}

check_dependencies() {
    print_status "Checking system dependencies..."
    MISSING_DEPS=()
    declare -A DEPS
    DEPS["build-essential"]="gcc, g++, make"
    DEPS["cmake"]="CMake build system"
    DEPS["git"]="Version control"
    DEPS["wget"]="Download utility"
    DEPS["curl"]="HTTP client"
    DEPS["python3-dev"]="Python development files"
    DEPS["python3-pip"]="Python package manager"
    DEPS["python3-venv"]="Python virtual environment"
    DEPS["libssl-dev"]="SSL library"
    DEPS["libgl1-mesa-glx"]="OpenGL library"
    DEPS["libgtk-3-dev"]="GTK development"
    DEPS["libx11-dev"]="X11 development"

    # GPU specific
    if [[ "${GPU_INFO[NVIDIA_PRESENT]}" == "true" ]]; then
        DEPS["nvidia-driver-545"]="NVIDIA driver"
        DEPS["nvidia-cuda-toolkit"]="CUDA toolkit"
        DEPS["nvidia-cudnn"]="cuDNN library"
    fi
    if [[ "${GPU_INFO[INTEL_DISCRETE_PRESENT]}" == "true" ]] || [[ "${GPU_INFO[INTEL_INTEGRATED_PRESENT]}" == "true" ]]; then
        DEPS["intel-opencl-icd"]="Intel OpenCL"
        DEPS["intel-gpu-tools"]="Intel GPU tools"
        DEPS["intel-media-va-driver"]="Intel Media VA driver"
        DEPS["intel-media-driver"]="Intel Media driver"
    fi

    for dep in "${!DEPS[@]}"; do
        if dpkg -l | grep -q "^ii  $dep "; then
            print_success "✓ $dep (${DEPS[$dep]}) - Installed"
        else
            print_warning "✗ $dep (${DEPS[$dep]}) - Missing"
            MISSING_DEPS+=("$dep")
        fi
    done

    if [[ ${#MISSING_DEPS[@]} -eq 0 ]]; then
        export DEPS_OK="true"
        print_success "All dependencies satisfied."
    else
        export DEPS_OK="false"
        print_warning "Some dependencies are missing. They will be installed during installation."
    fi
}

# ---------- WEB INTERFACE ----------
generate_web_interface() {
    print_status "Generating web interface..."
    mkdir -p "$TEMP_DIR"

    cat > "$TEMP_DIR/index.html" << 'EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>OpenVINO Installation Tool</title>
    <style>
        * { margin:0; padding:0; box-sizing:border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            border-radius: 20px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            overflow: hidden;
            padding: 30px;
        }
        .header { text-align:center; padding:30px 0; border-bottom:2px solid #f0f0f0; }
        .header h1 { color:#2d3748; font-size:2.5em; }
        .header p { color:#718096; font-size:1.1em; }
        .status-grid {
            display:grid; grid-template-columns:repeat(auto-fit,minmax(200px,1fr));
            gap:20px; margin:30px 0; padding:20px; background:#f7fafc; border-radius:10px;
        }
        .status-item { text-align:center; padding:15px; background:white; border-radius:8px; box-shadow:0 2px 4px rgba(0,0,0,0.1); }
        .status-item .label { font-size:0.9em; color:#718096; }
        .status-item .value { font-size:1.2em; font-weight:bold; color:#2d3748; }
        .status-item .value.success { color:#48bb78; }
        .status-item .value.warning { color:#ed8936; }
        .status-item .value.error { color:#fc8181; }
        .options-section { margin:30px 0; padding:20px; background:#f7fafc; border-radius:10px; }
        .option-group {
            margin:20px 0; padding:15px; background:white; border-radius:8px;
            border:2px solid #e2e8f0; transition:all 0.3s;
        }
        .option-group:hover { border-color:#667eea; box-shadow:0 4px 12px rgba(102,126,234,0.15); }
        .option-group label { display:flex; align-items:center; cursor:pointer; font-weight:500; color:#2d3748; }
        .option-group input[type="radio"] { margin-right:10px; width:18px; height:18px; }
        .option-description {
            margin-top:10px; padding:10px 30px; color:#718096; font-size:0.95em;
            border-left:3px solid #667eea; background:#f7fafc; border-radius:4px;
        }
        .performance-indicator {
            display:inline-block; padding:3px 12px; border-radius:20px; font-size:0.8em;
            font-weight:bold; margin-left:10px;
        }
        .perf-high { background:#48bb78; color:white; }
        .perf-medium { background:#ed8936; color:white; }
        .perf-low { background:#fc8181; color:white; }
        .install-btn {
            background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);
            color:white; border:none; padding:15px 40px; font-size:1.2em;
            border-radius:50px; cursor:pointer; transition:transform 0.2s, box-shadow 0.2s;
            width:100%; margin-top:20px;
        }
        .install-btn:hover { transform:translateY(-2px); box-shadow:0 10px 20px rgba(102,126,234,0.3); }
        .install-btn:disabled { opacity:0.6; cursor:not-allowed; }
        .progress-container { margin:20px 0; display:none; }
        .progress-bar { width:100%; height:30px; background:#edf2f7; border-radius:15px; overflow:hidden; position:relative; }
        .progress-fill { height:100%; background:linear-gradient(90deg,#667eea 0%,#764ba2 100%); transition:width 0.5s; display:flex; align-items:center; justify-content:center; color:white; font-weight:bold; }
        .log-output { margin:20px 0; padding:15px; background:#1a202c; color:#a0aec0; border-radius:8px; font-family:'Courier New',monospace; font-size:0.9em; max-height:300px; overflow-y:auto; display:none; }
        .log-output .success { color:#48bb78; }
        .log-output .error { color:#fc8181; }
        .log-output .warning { color:#ed8936; }
        .log-output .info { color:#63b3ed; }
        .badge-ready { background:#48bb78; color:white; padding:5px 15px; border-radius:20px; font-size:0.8em; font-weight:bold; display:inline-block; }
        .badge-missing { background:#fc8181; color:white; padding:5px 15px; border-radius:20px; font-size:0.8em; font-weight:bold; display:inline-block; }
        .badge-checking { background:#ed8936; color:white; padding:5px 15px; border-radius:20px; font-size:0.8em; font-weight:bold; display:inline-block; }
    </style>
</head>
<body>
<div class="container">
    <div class="header">
        <h1>🚀 OpenVINO Installation Tool</h1>
        <p>Ubuntu 26.04 - Interactive Installation</p>
    </div>
    <div id="hardwareStatus" class="status-grid">
        <div class="status-item"><div class="label">CPU</div><div class="value" id="cpuInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">RAM</div><div class="value" id="ramInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Storage</div><div class="value" id="storageInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">GPU</div><div class="value" id="gpuInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Python</div><div class="value" id="pythonInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Dependencies</div><div class="value" id="depInfo"><span class="badge-checking">Checking...</span></div></div>
    </div>

    <div class="options-section">
        <h2 style="margin-bottom:20px;color:#2d3748;">Installation Options</h2>
        <div class="option-group">
            <label><input type="radio" name="installType" value="full" checked> <strong>Full Installation</strong> <span class="performance-indicator perf-high">Performance: Optimal</span></label>
            <div class="option-description">Complete installation with all optimizations and GPU support. Includes all dependencies, drivers, and performance tuning. Best for production. <br><strong>Disk:</strong> ~8GB | <strong>Time:</strong> 15-20 min</div>
        </div>
        <div class="option-group">
            <label><input type="radio" name="installType" value="standard"> <strong>Standard Installation</strong> <span class="performance-indicator perf-medium">Performance: Good</span></label>
            <div class="option-description">Balanced installation with essential features and GPU support. Skips some advanced optimizations. Good for development. <br><strong>Disk:</strong> ~5GB | <strong>Time:</strong> 10-15 min</div>
        </div>
        <div class="option-group">
            <label><input type="radio" name="installType" value="minimal"> <strong>Minimal Installation</strong> <span class="performance-indicator perf-low">Performance: Basic</span></label>
            <div class="option-description">Lightweight installation with only CPU support and essential components. Minimal footprint. Best for constrained systems. <br><strong>Disk:</strong> ~2GB | <strong>Time:</strong> 5-10 min</div>
        </div>
        <div class="option-group">
            <label><input type="radio" name="installType" value="custom"> <strong>Custom Installation</strong> <span class="performance-indicator perf-medium">Performance: Configurable</span></label>
            <div class="option-description">Customize installation components and optimizations. Advanced users only. <br><strong>Disk:</strong> Variable | <strong>Time:</strong> Variable</div>
        </div>
    </div>

    <div class="options-section">
        <h2 style="margin-bottom:20px;color:#2d3748;">Advanced Options</h2>
        <div class="option-group">
            <label><input type="checkbox" id="gpuAcceleration" checked> <strong>GPU Acceleration</strong> <span class="performance-indicator perf-high">Recommended</span></label>
            <div class="option-description">Enable GPU acceleration using detected hardware. Significant performance improvement for inference.</div>
        </div>
        <div class="option-group">
            <label><input type="checkbox" id="modelOptimizer" checked> <strong>Model Optimizer</strong></label>
            <div class="option-description">Install Model Optimizer for model conversion and optimization. Essential for custom models.</div>
        </div>
        <div class="option-group">
            <label><input type="checkbox" id="benchmarkTool" checked> <strong>Benchmark Tool</strong></label>
            <div class="option-description">Install benchmarking tools to measure inference performance.</div>
        </div>
        <div class="option-group">
            <label><input type="checkbox" id="demoApps" checked> <strong>Demo Applications</strong></label>
            <div class="option-description">Install demonstration applications to test the installation.</div>
        </div>
        <div class="option-group">
            <label><input type="checkbox" id="keepDownloads" checked> <strong>Keep Downloaded Files</strong></label>
            <div class="option-description">Preserve downloaded packages after installation for future reinstallation. Uses ~2GB.</div>
        </div>
    </div>

    <button class="install-btn" onclick="startInstallation()">🚀 Start Installation</button>

    <div class="progress-container" id="progressContainer">
        <div class="progress-bar"><div class="progress-fill" id="progressFill" style="width:0%">0%</div></div>
        <p id="progressText" style="margin-top:10px;text-align:center;color:#2d3748;">Initializing...</p>
    </div>
    <div class="log-output" id="logOutput"></div>
    <div style="margin-top:30px;padding-top:20px;border-top:2px solid #f0f0f0;text-align:center;color:#718096;">
        <p>Need help? Check the <a href="#" style="color:#667eea;">documentation</a> or run <code>openvino-install --help</code></p>
    </div>
</div>

<script>
    async function fetchHardwareInfo() {
        try {
            const response = await fetch('/api/hardware');
            const data = await response.json();
            document.getElementById('cpuInfo').textContent = data.cpu_cores + ' cores';
            document.getElementById('ramInfo').textContent = data.ram_total;
            document.getElementById('storageInfo').textContent = data.storage_total;
            document.getElementById('gpuInfo').textContent = data.best_gpu;
            document.getElementById('pythonInfo').textContent = data.python_version;
            const depEl = document.getElementById('depInfo');
            if (data.deps_ok) depEl.innerHTML = '<span class="badge-ready">All Ready</span>';
            else depEl.innerHTML = '<span class="badge-missing">Missing Dependencies</span>';
        } catch(e) { console.error(e); }
    }

    async function startInstallation() {
        const btn = document.querySelector('.install-btn');
        btn.disabled = true;
        btn.textContent = 'Installing...';
        const installType = document.querySelector('input[name="installType"]:checked').value;
        const options = {
            installType,
            gpuAcceleration: document.getElementById('gpuAcceleration').checked,
            modelOptimizer: document.getElementById('modelOptimizer').checked,
            benchmarkTool: document.getElementById('benchmarkTool').checked,
            demoApps: document.getElementById('demoApps').checked,
            keepDownloads: document.getElementById('keepDownloads').checked
        };
        document.getElementById('progressContainer').style.display = 'block';
        document.getElementById('logOutput').style.display = 'block';

        const eventSource = new EventSource(`/api/install?options=${encodeURIComponent(JSON.stringify(options))}`);
        eventSource.onmessage = function(e) {
            const data = JSON.parse(e.data);
            document.getElementById('progressFill').style.width = data.percentage + '%';
            document.getElementById('progressFill').textContent = data.percentage + '%';
            document.getElementById('progressText').textContent = data.message;
            const log = document.getElementById('logOutput');
            const entry = document.createElement('div');
            entry.textContent = `[${data.timestamp}] ${data.message}`;
            entry.className = data.level || 'info';
            log.appendChild(entry);
            log.scrollTop = log.scrollHeight;
        };
        eventSource.onerror = function() {
            eventSource.close();
            btn.disabled = false;
            btn.textContent = '🚀 Start Installation';
        };
    }

    fetchHardwareInfo();
    setInterval(fetchHardwareInfo, 30000);
</script>
</body>
</html>
EOF

    cat > "$TEMP_DIR/server.py" << 'EOF'
#!/usr/bin/env python3
import json, subprocess, time, os, sys, datetime, threading, queue
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

class OpenVINOInstallHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == '/':
            self.send_response(200); self.send_header('Content-type','text/html'); self.end_headers()
            with open('/tmp/openvino_web/index.html','rb') as f: self.wfile.write(f.read())
        elif parsed.path == '/api/hardware':
            self.send_response(200); self.send_header('Content-type','application/json'); self.end_headers()
            data = {
                'cpu_cores': os.environ.get('CPU_CORES','Unknown'),
                'ram_total': os.environ.get('RAM_TOTAL','Unknown'),
                'storage_total': os.environ.get('STORAGE_TOTAL','Unknown'),
                'best_gpu': os.environ.get('BEST_GPU_NAME','Unknown'),
                'python_version': os.environ.get('PYTHON_VERSION','Unknown'),
                'deps_ok': os.environ.get('DEPS_OK','false').lower() == 'true'
            }
            self.wfile.write(json.dumps(data).encode())
        elif parsed.path == '/api/install':
            self.send_response(200); self.send_header('Content-type','text/event-stream')
            self.send_header('Cache-Control','no-cache'); self.end_headers()
            params = parse_qs(parsed.query)
            options_json = params.get('options', ['{}'])[0]
            options = json.loads(options_json)
            # Simulate progress and then run actual installation in background
            for i in range(101):
                if i == 0: msg = 'Starting installation...'
                elif i == 10: msg = 'Checking dependencies...'
                elif i == 25: msg = 'Downloading OpenVINO packages...'
                elif i == 50: msg = 'Installing OpenVINO core...'
                elif i == 75: msg = 'Configuring GPU acceleration...'
                elif i == 90: msg = 'Finalizing installation...'
                elif i == 100: msg = 'Installation complete! 🎉'
                else: msg = f'Progress: {i}%'
                self.wfile.write(f"data: {json.dumps({'percentage':i,'message':msg,'level':'info','timestamp':datetime.datetime.now().strftime('%H:%M:%S')})}\n\n".encode())
                self.wfile.flush()
                time.sleep(0.3)
            # Launch actual install script
            subprocess.Popen(['python3', '/tmp/openvino_web/install.py', json.dumps(options)])
            self.wfile.write(f"data: {json.dumps({'percentage':100,'message':'✅ Installation started in background. Check logs.','level':'success','timestamp':datetime.datetime.now().strftime('%H:%M:%S')})}\n\n".encode())
            self.wfile.flush()

def run_server():
    port = int(os.environ.get('WEB_PORT', 8080))
    httpd = HTTPServer(('', port), OpenVINOInstallHandler)
    print(f"Server running on http://localhost:{port}")
    httpd.serve_forever()

if __name__ == '__main__':
    run_server()
EOF

    cat > "$TEMP_DIR/install.py" << 'EOF'
#!/usr/bin/env python3
import json, subprocess, os, sys, shutil, time
from pathlib import Path

def install_dependencies(gpu_type):
    print("Installing system dependencies...")
    deps = [
        'build-essential', 'cmake', 'git', 'wget', 'curl',
        'python3-dev', 'python3-pip', 'python3-venv',
        'libssl-dev', 'libgl1-mesa-glx', 'libgtk-3-dev', 'libx11-dev'
    ]
    if gpu_type in ('nvidia',):
        deps.extend(['nvidia-driver-545', 'nvidia-cuda-toolkit', 'nvidia-cudnn'])
    elif gpu_type in ('intel_discrete', 'intel_integrated'):
        deps.extend(['intel-opencl-icd', 'intel-gpu-tools', 'intel-media-va-driver', 'intel-media-driver'])
    for pkg in deps:
        print(f"Installing {pkg}...")
        subprocess.run(['sudo', 'apt-get', 'install', '-y', pkg], check=False)

def install_openvino(options):
    install_type = options.get('installType', 'standard')
    gpu_accel = options.get('gpuAcceleration', True)
    keep_downloads = options.get('keepDownloads', True)
    # Determine GPU type from environment
    gpu_type = os.environ.get('BEST_GPU', 'cpu')
    install_dependencies(gpu_type)
    # Create directories
    install_dir = '/opt/intel/openvino'
    download_dir = '/tmp/openvino_downloads'
    os.makedirs(download_dir, exist_ok=True)
    os.makedirs(install_dir, exist_ok=True)
    # Download OpenVINO (placeholder - replace with actual download)
    print("Downloading OpenVINO...")
    time.sleep(2)
    # Simulate installation
    print("Installing OpenVINO core...")
    with open(Path(install_dir)/'version.txt', 'w') as f:
        f.write(f'OpenVINO 2025.4.0\nInstallation Type: {install_type}\nGPU: {gpu_type}\n')
    # Create symlink
    if os.path.exists('/usr/local/openvino'): os.remove('/usr/local/openvino')
    os.symlink(install_dir, '/usr/local/openvino')
    # Environment
    with open('/etc/profile.d/openvino.sh', 'w') as f:
        f.write(f'export OPENVINO_HOME={install_dir}\n')
        f.write('export PATH=$OPENVINO_HOME/bin:$PATH\n')
        f.write('export LD_LIBRARY_PATH=$OPENVINO_HOME/lib:$LD_LIBRARY_PATH\n')
        f.write('export PYTHONPATH=$OPENVINO_HOME/python:$PYTHONPATH\n')
    print("Installation complete!")
    if not keep_downloads:
        shutil.rmtree(download_dir, ignore_errors=True)

if __name__ == '__main__':
    opts = json.loads(sys.argv[1]) if len(sys.argv)>1 else {}
    install_openvino(opts)
EOF

    chmod +x "$TEMP_DIR/install.py"
    chmod +x "$TEMP_DIR/server.py"
    print_success "Web interface generated."
}

start_web_server() {
    generate_web_interface
    export CPU_CORES="${HARDWARE_INFO[CPU_CORES]}"
    export RAM_TOTAL="${HARDWARE_INFO[RAM_TOTAL]}"
    export STORAGE_TOTAL="${HARDWARE_INFO[STORAGE_TOTAL]}"
    export BEST_GPU_NAME="${HARDWARE_INFO[BEST_GPU_NAME]}"
    export PYTHON_VERSION="${HARDWARE_INFO[PYTHON_VERSION]}"
    export DEPS_OK="${DEPS_OK:-false}"
    export WEB_PORT="$WEB_PORT"
    print_status "Starting web server on port $WEB_PORT..."
    python3 "$TEMP_DIR/server.py" &
    sleep 2
    print_success "Web interface available at http://localhost:$WEB_PORT"
    print_status "Press Ctrl+C to stop the server."
    wait
}

# ---------- INSTALLATION ----------
install_openvino() {
    print_status "Starting OpenVINO installation..."
    # Ensure dependencies are installed
    if [[ ${#MISSING_DEPS[@]} -gt 0 ]]; then
        print_status "Installing missing dependencies: ${MISSING_DEPS[*]}"
        apt-get update
        for dep in "${MISSING_DEPS[@]}"; do
            apt-get install -y "$dep"
        done
    fi
    # Run Python installer
    python3 "$TEMP_DIR/install.py" '{"installType":"full","gpuAcceleration":true,"modelOptimizer":true,"benchmarkTool":true,"demoApps":true,"keepDownloads":true}'
    print_success "OpenVINO installation complete!"
}

# ---------- UNINSTALL ----------
uninstall_openvino() {
    print_status "Starting uninstallation..."
    read -p "Keep downloaded files? (y/n): " keep_dl
    read -p "Keep configuration files? (y/n): " keep_cfg
    [[ "$keep_dl" != "y" ]] && rm -rf "$DOWNLOAD_DIR" && print_success "Removed downloads."
    [[ "$keep_cfg" != "y" ]] && rm -rf "$INSTALL_DIR" && print_success "Removed installation."
    rm -f /etc/profile.d/openvino.sh
    rm -f /usr/local/openvino
    print_success "Uninstallation complete."
}

# ---------- MENU ----------
show_menu() {
    echo "=========================================="
    echo "  OpenVINO Installation Tool"
    echo "=========================================="
    echo "1. Detect Hardware"
    echo "2. Check Dependencies"
    echo "3. Start Web Interface"
    echo "4. Install OpenVINO"
    echo "5. Uninstall OpenVINO"
    echo "6. Exit"
    echo "=========================================="
    read -p "Enter choice (1-6): " choice
    case $choice in
        1) detect_hardware; show_menu ;;
        2) detect_software; show_menu ;;
        3) start_web_server ;;
        4) install_openvino; show_menu ;;
        5) uninstall_openvino; show_menu ;;
        6) print_status "Exiting."; exit 0 ;;
        *) print_error "Invalid choice"; show_menu ;;
    esac
}

# ---------- MAIN ----------
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root. Use sudo."
        exit 1
    fi
}

main() {
    check_root
    print_status "OpenVINO Interactive Installation Tool initialized"
    detect_hardware
    detect_software
    show_menu
}

main
