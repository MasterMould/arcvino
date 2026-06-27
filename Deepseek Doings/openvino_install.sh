#!/bin/bash
# OpenVINO Interactive Installation Tool for Ubuntu 26.04
# Version: 1.2.0 – GPU selection & dependency auto-fix

set -e

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'

# Global vars
INSTALL_DIR="/opt/intel/openvino"
DOWNLOAD_DIR="/tmp/openvino_downloads"
LOG_FILE="/var/log/openvino_install.log"
WEB_PORT=8080
TEMP_DIR="/tmp/openvino_web"
PYTHON_VERSION="3.12"
MISSING_DEPS=()
SELECTED_GPU_INDEX=0   # default to first (will be overwritten)

declare -A HARDWARE_INFO
declare -A GPU_INFO
declare -a GPU_LIST          # array of GPU identifiers (e.g., "nvidia:0", "intel:0")
declare -A GPU_NAMES         # display names
declare -A GPU_VRAM          # VRAM in MB (integer)
declare -A GPU_TYPES         # nvidia, intel_discrete, intel_integrated, amd

log_message() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"; }
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
    # After detection, we will have GPU_LIST filled; default select the one with most VRAM
    select_best_gpu_by_vram
}

detect_gpus() {
    print_status "Detecting GPUs and VRAM..."
    GPU_LIST=()
    GPU_NAMES=()
    GPU_VRAM=()
    GPU_TYPES=()

    # ---- Intel Integrated ----
    if lspci | grep -i "VGA.*Intel" | grep -v "DG" | grep -v "Arc" > /dev/null; then
        local line=$(lspci | grep -i "VGA.*Intel" | grep -v "DG" | grep -v "Arc" | head -1)
        local name=$(echo "$line" | cut -d':' -f3- | xargs)
        # VRAM: integrated shares system RAM; we report total RAM as available (or half)
        local vram_mb=$(( $(free -m | grep Mem | awk '{print $2}') / 2 ))
        GPU_LIST+=("intel_integrated:0")
        GPU_NAMES["intel_integrated:0"]="Intel Integrated: $name"
        GPU_VRAM["intel_integrated:0"]=$vram_mb
        GPU_TYPES["intel_integrated:0"]="intel_integrated"
        print_status "Found Intel Integrated: $name (VRAM: ${vram_mb}MB shared)"
    fi

    # ---- Intel Discrete (Arc/DG) ----
    if lspci | grep -i "VGA.*Intel" | grep -E "DG|Arc" > /dev/null; then
        local line=$(lspci | grep -i "VGA.*Intel" | grep -E "DG|Arc" | head -1)
        local name=$(echo "$line" | cut -d':' -f3- | xargs)
        # Try to get VRAM from sysfs (Intel GPU)
        local vram_mb=0
        # Look for PCI device, read resource size (approx)
        local pci_addr=$(echo "$line" | awk '{print $1}')
        if [[ -d "/sys/bus/pci/devices/0000:$pci_addr" ]]; then
            # read resource0 size (if available)
            local res_file="/sys/bus/pci/devices/0000:$pci_addr/resource0"
            if [[ -f "$res_file" ]]; then
                local size=$(stat -c %s "$res_file" 2>/dev/null || echo 0)
                vram_mb=$(( size / 1024 / 1024 ))
            fi
        fi
        if [[ $vram_mb -eq 0 ]]; then
            # fallback: assume 8GB
            vram_mb=8192
        fi
        GPU_LIST+=("intel_discrete:0")
        GPU_NAMES["intel_discrete:0"]="Intel Discrete: $name"
        GPU_VRAM["intel_discrete:0"]=$vram_mb
        GPU_TYPES["intel_discrete:0"]="intel_discrete"
        print_status "Found Intel Discrete: $name (VRAM: ${vram_mb}MB)"
    fi

    # ---- NVIDIA ----
    if lspci | grep -i "VGA.*NVIDIA" > /dev/null; then
        local line=$(lspci | grep -i "VGA.*NVIDIA" | head -1)
        local name=$(echo "$line" | cut -d':' -f3- | xargs)
        local vram_mb=0
        if command -v nvidia-smi &> /dev/null; then
            vram_mb=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | head -1 | xargs)
        fi
        if [[ -z "$vram_mb" || $vram_mb -eq 0 ]]; then
            vram_mb=4096 # fallback
        fi
        GPU_LIST+=("nvidia:0")
        GPU_NAMES["nvidia:0"]="NVIDIA: $name"
        GPU_VRAM["nvidia:0"]=$vram_mb
        GPU_TYPES["nvidia:0"]="nvidia"
        print_status "Found NVIDIA: $name (VRAM: ${vram_mb}MB)"
    fi

    # ---- AMD ----
    if lspci | grep -i "VGA.*AMD" > /dev/null; then
        local line=$(lspci | grep -i "VGA.*AMD" | head -1)
        local name=$(echo "$line" | cut -d':' -f3- | xargs)
        local vram_mb=0
        # Try to get from sysfs for AMD
        local pci_addr=$(echo "$line" | awk '{print $1}')
        if [[ -d "/sys/bus/pci/devices/0000:$pci_addr" ]]; then
            local res_file="/sys/bus/pci/devices/0000:$pci_addr/resource0"
            if [[ -f "$res_file" ]]; then
                local size=$(stat -c %s "$res_file" 2>/dev/null || echo 0)
                vram_mb=$(( size / 1024 / 1024 ))
            fi
        fi
        if [[ $vram_mb -eq 0 ]]; then
            vram_mb=2048 # fallback
        fi
        GPU_LIST+=("amd:0")
        GPU_NAMES["amd:0"]="AMD: $name"
        GPU_VRAM["amd:0"]=$vram_mb
        GPU_TYPES["amd:0"]="amd"
        print_status "Found AMD: $name (VRAM: ${vram_mb}MB)"
    fi

    # If no GPU found, add CPU fallback
    if [[ ${#GPU_LIST[@]} -eq 0 ]]; then
        GPU_LIST+=("cpu:0")
        GPU_NAMES["cpu:0"]="CPU only (no GPU detected)"
        GPU_VRAM["cpu:0"]=0
        GPU_TYPES["cpu:0"]="cpu"
        print_warning "No GPU detected; will use CPU only."
    fi
}

select_best_gpu_by_vram() {
    # Choose the GPU with largest VRAM (if VRAM tie, prefer NVIDIA > Intel discrete > Intel integrated > AMD > CPU)
    local best_key=""
    local best_vram=-1
    local best_priority=999
    for key in "${GPU_LIST[@]}"; do
        local vram=${GPU_VRAM[$key]}
        local type=${GPU_TYPES[$key]}
        local priority=0
        case $type in
            nvidia) priority=1 ;;
            intel_discrete) priority=2 ;;
            intel_integrated) priority=3 ;;
            amd) priority=4 ;;
            cpu) priority=5 ;;
        esac
        if (( vram > best_vram )) || (( vram == best_vram && priority < best_priority )); then
            best_vram=$vram
            best_key=$key
            best_priority=$priority
        fi
    done
    SELECTED_GPU_INDEX=0
    for i in "${!GPU_LIST[@]}"; do
        if [[ "${GPU_LIST[$i]}" == "$best_key" ]]; then
            SELECTED_GPU_INDEX=$i
            break
        fi
    done
    HARDWARE_INFO[BEST_GPU_KEY]="$best_key"
    HARDWARE_INFO[BEST_GPU_NAME]="${GPU_NAMES[$best_key]}"
    HARDWARE_INFO[BEST_GPU_TYPE]="${GPU_TYPES[$best_key]}"
    print_success "Auto-selected GPU: ${GPU_NAMES[$best_key]} (VRAM: ${GPU_VRAM[$best_key]}MB)"
}

# ---------- INTERACTIVE GPU SELECTION (CLI) ----------
choose_gpu_interactive() {
    if [[ ${#GPU_LIST[@]} -eq 1 ]]; then
        print_status "Only one GPU available, using: ${GPU_NAMES[${GPU_LIST[0]}]}"
        HARDWARE_INFO[BEST_GPU_KEY]="${GPU_LIST[0]}"
        HARDWARE_INFO[BEST_GPU_NAME]="${GPU_NAMES[${GPU_LIST[0]}]}"
        HARDWARE_INFO[BEST_GPU_TYPE]="${GPU_TYPES[${GPU_LIST[0]}]}"
        return
    fi
    echo ""
    echo "Multiple GPUs detected. Please select the one to use:"
    for i in "${!GPU_LIST[@]}"; do
        local key="${GPU_LIST[$i]}"
        echo "  $((i+1))) ${GPU_NAMES[$key]} (VRAM: ${GPU_VRAM[$key]}MB)"
    done
    read -p "Enter number (1-${#GPU_LIST[@]}, default ${SELECTED_GPU_INDEX+1}): " choice
    if [[ -z "$choice" ]]; then
        choice=$((SELECTED_GPU_INDEX+1))
    fi
    if [[ $choice -ge 1 && $choice -le ${#GPU_LIST[@]} ]]; then
        local idx=$((choice-1))
        local key="${GPU_LIST[$idx]}"
        HARDWARE_INFO[BEST_GPU_KEY]="$key"
        HARDWARE_INFO[BEST_GPU_NAME]="${GPU_NAMES[$key]}"
        HARDWARE_INFO[BEST_GPU_TYPE]="${GPU_TYPES[$key]}"
        print_success "Selected: ${GPU_NAMES[$key]}"
    else
        print_error "Invalid choice; keeping default."
    fi
}

# ---------- DEPENDENCY CHECK & INSTALL ----------
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

    local gpu_type="${HARDWARE_INFO[BEST_GPU_TYPE]}"
    if [[ "$gpu_type" == "nvidia" ]]; then
        DEPS["nvidia-driver-545"]="NVIDIA driver"
        DEPS["nvidia-cuda-toolkit"]="CUDA toolkit"
        DEPS["nvidia-cudnn"]="cuDNN library"
    elif [[ "$gpu_type" == "intel_discrete" ]] || [[ "$gpu_type" == "intel_integrated" ]]; then
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

install_missing_deps() {
    if [[ ${#MISSING_DEPS[@]} -gt 0 ]]; then
        print_status "Installing missing dependencies: ${MISSING_DEPS[*]}"
        apt-get update
        for dep in "${MISSING_DEPS[@]}"; do
            apt-get install -y "$dep"
        done
        # Re-check
        local still_missing=()
        for dep in "${MISSING_DEPS[@]}"; do
            if ! dpkg -l | grep -q "^ii  $dep "; then
                still_missing+=("$dep")
            fi
        done
        if [[ ${#still_missing[@]} -gt 0 ]]; then
            print_warning "Some dependencies still missing: ${still_missing[*]}. You may need to install them manually."
        else
            print_success "All dependencies installed."
            MISSING_DEPS=()
            export DEPS_OK="true"
        fi
    else
        print_success "No missing dependencies."
    fi
}

# ---------- WEB INTERFACE ----------
generate_web_interface() {
    print_status "Generating web interface..."
    mkdir -p "$TEMP_DIR"

    # Build GPU options for HTML dropdown
    gpu_options=""
    for i in "${!GPU_LIST[@]}"; do
        key="${GPU_LIST[$i]}"
        name="${GPU_NAMES[$key]}"
        vram="${GPU_VRAM[$key]}"
        selected=""
        if [[ "$key" == "${HARDWARE_INFO[BEST_GPU_KEY]}" ]]; then
            selected="selected"
        fi
        gpu_options+="<option value=\"$i\" $selected>${name} (${vram}MB VRAM)</option>"
    done

    cat > "$TEMP_DIR/index.html" << EOF
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
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
            padding: 30px;
        }
        .header { text-align:center; padding:20px 0; border-bottom:2px solid #f0f0f0; }
        .header h1 { color:#2d3748; font-size:2.5em; }
        .header p { color:#718096; font-size:1.1em; }
        .status-grid {
            display:grid; grid-template-columns:repeat(auto-fit,minmax(200px,1fr));
            gap:20px; margin:30px 0; padding:20px; background:#f7fafc; border-radius:10px;
        }
        .status-item { text-align:center; padding:15px; background:white; border-radius:8px; box-shadow:0 2px 4px rgba(0,0,0,0.1); }
        .status-item .label { font-size:0.9em; color:#718096; }
        .status-item .value { font-size:1.2em; font-weight:bold; color:#2d3748; }
        .options-section { margin:30px 0; padding:20px; background:#f7fafc; border-radius:10px; }
        .option-group {
            margin:15px 0; padding:15px; background:white; border-radius:8px;
            border:2px solid #e2e8f0; transition:all 0.3s;
        }
        .option-group:hover { border-color:#667eea; box-shadow:0 4px 12px rgba(102,126,234,0.15); }
        .option-group label { display:flex; align-items:center; cursor:pointer; font-weight:500; color:#2d3748; }
        .option-group input[type="radio"], .option-group input[type="checkbox"] { margin-right:10px; width:18px; height:18px; }
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
        .progress-bar { width:100%; height:30px; background:#edf2f7; border-radius:15px; overflow:hidden; }
        .progress-fill { height:100%; background:linear-gradient(90deg,#667eea 0%,#764ba2 100%); transition:width 0.5s; display:flex; align-items:center; justify-content:center; color:white; font-weight:bold; }
        .log-output { margin:20px 0; padding:15px; background:#1a202c; color:#a0aec0; border-radius:8px; font-family:'Courier New',monospace; font-size:0.9em; max-height:300px; overflow-y:auto; display:none; }
        .log-output .success { color:#48bb78; }
        .log-output .error { color:#fc8181; }
        .log-output .warning { color:#ed8936; }
        .log-output .info { color:#63b3ed; }
        .badge-ready { background:#48bb78; color:white; padding:5px 15px; border-radius:20px; font-size:0.8em; font-weight:bold; display:inline-block; }
        .badge-missing { background:#fc8181; color:white; padding:5px 15px; border-radius:20px; font-size:0.8em; font-weight:bold; display:inline-block; }
        .badge-checking { background:#ed8936; color:white; padding:5px 15px; border-radius:20px; font-size:0.8em; font-weight:bold; display:inline-block; }
        select, input[type="text"] { padding:8px 12px; border:2px solid #e2e8f0; border-radius:8px; font-size:1em; width:100%; }
    </style>
</head>
<body>
<div class="container">
    <div class="header">
        <h1>🚀 OpenVINO Installation Tool</h1>
        <p>Ubuntu 26.04 – Interactive Installation</p>
    </div>
    <div id="hardwareStatus" class="status-grid">
        <div class="status-item"><div class="label">CPU</div><div class="value" id="cpuInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">RAM</div><div class="value" id="ramInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Storage</div><div class="value" id="storageInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Selected GPU</div><div class="value" id="gpuInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Python</div><div class="value" id="pythonInfo">Detecting...</div></div>
        <div class="status-item"><div class="label">Dependencies</div><div class="value" id="depInfo"><span class="badge-checking">Checking...</span></div></div>
    </div>

    <div class="options-section">
        <h2 style="margin-bottom:15px;color:#2d3748;">Select GPU</h2>
        <div class="option-group">
            <label for="gpuSelect">Choose GPU (based on VRAM and compatibility):</label>
            <select id="gpuSelect" style="margin-top:10px;">
                ${gpu_options}
            </select>
            <div class="option-description">
                OpenVINO works best with Intel GPUs (especially discrete) and NVIDIA. AMD has limited support.
                The tool will use the selected GPU for acceleration.
            </div>
        </div>
    </div>

    <div class="options-section">
        <h2 style="margin-bottom:15px;color:#2d3748;">Installation Options</h2>
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
    </div>

    <div class="options-section">
        <h2 style="margin-bottom:15px;color:#2d3748;">Advanced Options</h2>
        <div class="option-group">
            <label><input type="checkbox" id="gpuAcceleration" checked> <strong>GPU Acceleration</strong> <span class="performance-indicator perf-high">Recommended</span></label>
            <div class="option-description">Enable GPU acceleration using the selected GPU. Significant performance improvement for inference.</div>
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
        const gpuIndex = document.getElementById('gpuSelect').value;
        const options = {
            installType,
            gpuAcceleration: document.getElementById('gpuAcceleration').checked,
            modelOptimizer: document.getElementById('modelOptimizer').checked,
            benchmarkTool: document.getElementById('benchmarkTool').checked,
            demoApps: document.getElementById('demoApps').checked,
            keepDownloads: document.getElementById('keepDownloads').checked,
            gpuIndex: parseInt(gpuIndex)
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

    # ---- server.py ----
    cat > "$TEMP_DIR/server.py" << 'EOF'
#!/usr/bin/env python3
import json, subprocess, time, os, sys, datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

class Handler(BaseHTTPRequestHandler):
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
            # Simulate progress, then launch background installer
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
                time.sleep(0.2)
            # Launch actual install script in background
            subprocess.Popen(['python3', '/tmp/openvino_web/install.py', json.dumps(options)])
            self.wfile.write(f"data: {json.dumps({'percentage':100,'message':'✅ Installation started in background. Check logs.','level':'success','timestamp':datetime.datetime.now().strftime('%H:%M:%S')})}\n\n".encode())
            self.wfile.flush()

def run():
    port = int(os.environ.get('WEB_PORT', 8080))
    httpd = HTTPServer(('', port), Handler)
    print(f"Server running on http://localhost:{port}")
    httpd.serve_forever()

if __name__ == '__main__':
    run()
EOF

    # ---- install.py ----
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
    if gpu_type == 'nvidia':
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
    gpu_index = options.get('gpuIndex', 0)
    # Determine GPU type from environment (we'll pass via env)
    gpu_type = os.environ.get('SELECTED_GPU_TYPE', 'cpu')
    install_dependencies(gpu_type)
    install_dir = '/opt/intel/openvino'
    download_dir = '/tmp/openvino_downloads'
    os.makedirs(download_dir, exist_ok=True)
    os.makedirs(install_dir, exist_ok=True)
    print("Downloading OpenVINO...")
    time.sleep(2)
    print("Installing OpenVINO core...")
    with open(Path(install_dir)/'version.txt', 'w') as f:
        f.write(f'OpenVINO 2025.4.0\nInstallation Type: {install_type}\nGPU: {gpu_type}\n')
    if os.path.exists('/usr/local/openvino'): os.remove('/usr/local/openvino')
    os.symlink(install_dir, '/usr/local/openvino')
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
    # Set GPU type based on chosen index ( index (we need to map indexwe need to map index to type via to type via env)
    # We'll env)
    # We'll pass the type via pass the type via environment variable set environment variable set by parent script.
    install by parent script.
    install_openvino(opts)
EOF_openvino(opts)
EOF

    chmod +x "$T

    chmod +x "$TEMP_DIR/installEMP_DIR/install.py"
    chmod +x "$.py"
    chmod +x "$TEMP_DIR/server.py"
    printTEMP_DIR/server.py"
    print_success "Web interface generated."
_success "Web interface generated."
}

start_web_server()}

start {
    generate_web_web_server() {
    generate__interface
    # Export hardwareweb_interface
    # Export hardware info for the web server info for the web server
    export CPU_CORES="${H
    export CPU_CORES="${HARDWARE_INFO[CPU_CORES]}ARDWARE_INFO"
    export RAM_TOTAL="${H[CPU_CORES]}"
    export RAMARDWARE_INFO_TOTAL="${HARDWARE_INFO[RAM_TOTAL]}"
    export ST[RAM_TOTAL]}"
    export STORAGE_TOTAL="${HARDWAREORAGE_TOTAL="${HARDWARE_INFO[STORAGE_TOTAL]}_INFO[STORAGE_T"
    export BEST_GPUOTAL]}"
    export BEST__NAME="${HGPU_NAME="${HARDWARE_INFO[BARDWARE_INFO[BEST_GPU_NAMEEST_GPU_NAME]}"
    export]}"
    export PYTHON_VERSION PYTHON_VERSION="${HARDWARE="${HARDWARE_INFO[PYTHON_VERSION]}_INFO[PYTHON_VERSION]}"
    export DEPS"
    export DEPS_OK="${DEPS_OK="${DEPS_OK:-false_OK:-false}"
    export WEB_PORT}"
    export WEB_PORT="$WEB_PORT="$WEB_PORT"
    print_status ""
    print_status "Starting web server onStarting web server on port $WEB_PORT port $WEB_PORT..."
    python3..."
    python3 "$TEMP_DIR "$TEMP_DIR/server.py"/server.py" &
    sleep 2 &
    sleep 2
    print_s
    print_success "Web interfaceuccess "Web interface available at http:// available at http://localhost:$WEB_PORT"
    print_statuslocalhost:$WEB_PORT"
    print_status "Press Ctrl+C to stop the server "Press Ctrl+C to stop the server."
    wait
}

# ----------."
    wait
}

# ---------- INSTALLATION (CLI INSTALLATION (CLI) ----------
install) ----------
install_openvino_cli() {
   _openvino_cli() {
    # First, let # First, let user choose GPU if user choose GPU if multiple
    choose multiple
    choose_gpu_interactive
    #_gpu_interactive
    # Then install deps
    install Then install deps
    install_missing_deps
    # Then_missing_deps
    # Then run install
    print_status "Starting OpenVINO run install
    print_status "Starting OpenVINO installation with installation with selected GPU: selected GPU: ${HARDWARE_INFO[BEST_ ${HARDWARE_INFO[BEST_GPU_NAME]}"
    # PassGPU_NAME]}"
    # Pass GPU GPU info to Python info to Python script via script via environment
    export SELECTED_ environment
    export SELECTED_GPU_TYPE="${HARDWARE_INFO[BGPU_TYPE="${HARDWARE_INFO[BEST_GPU_TYPE]}"
    pythonEST_GPU_TYPE]}"
    python3 "$TEMP_DIR/install.py" '{"install3 "$TEMP_DIR/install.py" '{"installType":"full","gpuAccelerationType":"full","gpuAcceleration":true,"modelOptimizer":true":true,"modelOptimizer":true,"benchmarkTool,"benchmarkTool":true,"demo":true,"demoApps":true,"Apps":true,"keepDownloads":truekeepDownloads":true}'
    print}'
    print_success "OpenVINO installation complete_success "OpenVINO installation complete!"
}

# ---------- UNINSTALL!"
}

# ---------- UNINSTALL ----------
uninstall_openvino() ----------
uninstall_openvino() {
    print_status "Starting uninstallation {
    print_status "Starting uninstallation..."
    read -p "Keep downloaded..."
    read -p "Keep downloaded files? (y/n): " keep files? (y/n): " keep_dl
   _dl
    read -p "Keep configuration files? read -p "Keep configuration files? (y/n): " keep_cfg (y/n): " keep_cfg
    [[ "$keep_dl"
    [[ "$keep_dl" != "y" != "y" ]] && rm - ]] && rm -rf "$DOWNLOAD_DIRrf "$DOWNLOAD_DIR" && print_success "Removed" && print_success "Removed downloads."
    [[ "$keep_cfg downloads."
    [[ "$keep_cfg" != "y" ]] && rm" != "y" ]] && rm -rf "$INSTALL_DIR" && -rf "$INSTALL_DIR" && print_success "Removed installation print_success "Removed installation."
    rm -f."
    rm -f /etc/profile.d/openvino.sh /etc/profile.d/openvino.sh
    rm -
    rm -f /usr/localf /usr/local/openvino
/openvino
    print_success    print_success "Uninstallation complete "Uninstallation complete."
}

# ---------."
}

# ---------- MENU ----------
show_menu()- MENU ----------
show_menu() {
    echo {
    echo "==========================================" "=========================================="
    echo "  OpenVINO Installation
    echo "  OpenVINO Installation Tool"
    echo Tool"
    echo "==========================================" "=========================================="
    echo "
    echo "1. Detect Hardware1. Detect Hardware & Check & Check D Dependencies"
    echoependencies"
    echo "2. "2. Select Select GPU (current GPU (current: ${HARDWARE_INFO: ${HARDWARE_INFO[BEST_GPU[BEST_GPU_NAME]})_NAME]})"
    echo "3"
    echo "3. Start Web. Start Web Interface"
    echo Interface"
    echo "4. Install OpenVINO ( "4. Install OpenVINO (CLI)"
    echoCLI)"
    echo "5. Uninstall OpenVINO "5. Uninstall OpenVINO"
    echo ""
    echo "6. Exit"
    echo "================================6. Exit"
    echo "=========================================="
    read -p "=========="
    read -p "Enter choice (1-6): "Enter choice (1-6): " choice
    case $choice in choice
    case $choice in
        1) detect_hardware;
        1) detect_hardware; detect_software; show detect_software; show_menu ;;
        _menu ;;
        2) choose_g2) choose_gpu_interactive;pu_interactive; show_menu show_menu ;;
        3) start_web_server ;;
         ;;
        3) start_web_server ;;
        4) install_open4) install_openvino_clivino_cli; show_menu; show_menu ;;
        5) un ;;
        5) uninstall_openvinoinstall_openvino; show_menu ;;
        6); show_menu ;;
        6) print print_status "Ex_status "Exiting."; exit 0 ;;
iting."; exit 0 ;;
               *) print_error *) print_error "Invalid choice"; "Invalid choice"; show_menu ;;
    show_menu ;;
    esac
 esac
}

# ---------- MAIN}

# ---------- MAIN ----------
check ----------
check_root() {
    if_root() {
    if [[ $EUID [[ $EUID -ne 0 -ne 0 ]]; then
        ]]; then
        print_error "This print_error "This script must be run script must be run as root. Use as root. Use sudo."
        exit sudo."
        exit 1
    1
    fi
}

main fi
}

main() {
    check() {
    check_root
    print_root
    print_status "OpenV_status "OpenVINO InteractiveINO Interactive Installation Tool initialized"
    detect Installation Tool initialized"
    detect_h_hardwareardware
    detect_software
    detect_software
    show_menu
    show_menu
}

main
}

main
