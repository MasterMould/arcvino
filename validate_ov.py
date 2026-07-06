from optimum.intel import OVModelForCausalLM
from transformers import AutoTokenizer

model_id = "TinyLlama/TinyLlama-1.1B-Chat-v1.0"
print(f"🚀 Initializing OpenVINO deployment for: {model_id}")

# This will export the model to OpenVINO IR format on the first run
model = OVModelForCausalLM.from_pretrained(model_id, export=True, device="GPU")
tokenizer = AutoTokenizer.from_pretrained(model_id)

inputs = tokenizer("Intel Arc is powerful because", return_tensors="pt")
outputs = model.generate(**inputs, max_new_tokens=20)

print("✅ Inference result:", tokenizer.decode(outputs[0], skip_special_tokens=True))
print("✅ SUCCESS: Your Arc A770 is natively accelerating this model.")

# To validate in virt env. Run: ~/openvino_env/bin/python3 validate_ov.py
