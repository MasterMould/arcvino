#!/bin/bash
# kickstart.sh - Minimal bootstrap to install Go and launch the installer

set -e

echo "🚂 Starting the  Bootstrap..."

Setup_SYCL(){
# 1. Source the oneAPI environment variables (Required every time you build/run)
source /opt/intel/oneapi/setvars.sh --force

# 2. Clean previous build files
cd $USER/ai_stack/llama.cpp
rm -rf build
mkdir build
cd build

# 3. Configure with SYCL enabled
# We use 'icpx' (Intel's SYCL compiler) to build the binary
cmake .. -DGGML_SYCL=ON \
         -DCMAKE_C_COMPILER=icx \
         -DCMAKE_CXX_COMPILER=icpx \
         -DCMAKE_BUILD_TYPE=Release

# 4. Build
make -j$(nproc)

ldd bin/llama-cli | grep libsycl
}

Validation(){
# Example: Offload 99 layers (all layers) to the Arc A770
./bin/llama-cli -m $HOME/ai_stack/models/llama-3.2-1b.gguf -ngl 99 -p "Hello!"
}

Setup_go(){
# Add the expected installation path to the current script session immediately
export PATH=$PATH:/usr/local/go/bin

# Check if Go physically exists before trying to fetch it
if ! command -v go &> /dev/null; then
    echo "⚠️ Go is not detected in your current path. Checking installation..."
    
    if [ ! -x "/usr/local/go/bin/go" ]; then
        echo "📥 Fetching the latest stable compiler..."
        GO_VERSION="1.22.4" 
        wget -q --show-progress "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
        
        echo "🔧 Extracting Go to /usr/local (requires sudo)..."
        sudo rm -rf /usr/local/go 
        sudo tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
    fi
    
    # Ensure it's active for the rest of this specific run
    export PATH=$PATH:/usr/local/go/bin
    
    if ! grep -q "/usr/local/go/bin" ~/.profile; then
        echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.profile
    fi
else
    echo "✅ Go compiler verified: $(go version)"
fi
}

Setup_wizard(){
if [ ! -f "go.mod" ]; then
    echo "📦 Initializing Go workspace environment..."
    go mod init openvino-installer
fi

echo "🏗️ Compiling the OpenVINO hardware discovery engine..."
go build -o openvino-wizard main.go
}

Run_wizard(){
echo "🚀 Launching the local web interface..."
./openvino-wizard
}

Setup_SYCL

