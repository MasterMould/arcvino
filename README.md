# arcvino
Setup openvino for Intel Arc GPU.


arcus/
├── main.go                # Application entry point and router map
├── types.go               # Common structs (InstallOptions, LaunchOptions, etc.)
├── handlers_deploy.go     # Installation and uninstallation logic
├── handlers_runtime.go    # Engine initialization and process management
├── frontend/
│   └── index.html         # The complete UI (HTML/CSS/JS)
└── templates/
    ├── server.py.tmpl     # The isolated Python OpenVINO server code
    └── system.sh.tmpl     # Bash execution and environment wrappers
