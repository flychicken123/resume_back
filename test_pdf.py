#!/usr/bin/env python3
import json
import sys
import os

# Test data
test_data = {
    "htmlContent": "<html><body><h1>Test Resume</h1><p>This is a test resume.</p></body></html>"
}

# Convert to JSON string
json_data = json.dumps(test_data)

# Test the generate_resume.py script
result = os.system(f'python generate_resume.py temp1 "{json_data}" test_output.pdf')

if result == 0:
    print("✅ PDF generation test successful!")
    if os.path.exists('test_output.pdf'):
        print(f"✅ PDF file created: {os.path.getsize('test_output.pdf')} bytes")
    else:
        print("❌ PDF file not found")
else:
    print("❌ PDF generation test failed")
