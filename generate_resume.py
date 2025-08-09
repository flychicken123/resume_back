#!/usr/bin/env python3
import json
import os
import sys
import subprocess
from subprocess import run

def get_system_info():
    """Get system info for debugging Docker vs local differences"""
    info = {}
    try:
        # Get wkhtmltopdf version
        result = run(['wkhtmltopdf', '--version'], capture_output=True, text=True)
        info['wkhtmltopdf_version'] = result.stdout.strip() if result.returncode == 0 else result.stderr.strip()
    except Exception as e:
        info['wkhtmltopdf_version'] = f"Error: {e}"
    
    try:
        # Get available fonts
        result = run(['fc-list', '--format=%{family}\n'], capture_output=True, text=True)
        fonts = result.stdout.strip().split('\n') if result.returncode == 0 else []
        info['available_fonts'] = list(set(fonts))[:10]  # First 10 unique fonts
    except Exception as e:
        info['available_fonts'] = f"Error: {e}"
    
    info['python_version'] = sys.version
    info['platform'] = sys.platform
    return info

def generate_html_resume(template_name, user_data, output_path):
    html_content = user_data.get('htmlContent', '')
    if not html_content:
        return False, "HTML content is required"
    try:
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(html_content)
        return True, None
    except Exception as e:
        return False, str(e)

def generate_pdf_resume(template_name, user_data, output_path):
    # First generate HTML
    html_path = output_path.replace('.pdf', '.html')
    success, error = generate_html_resume(template_name, user_data, html_path)
    if not success:
        return False, error

    try:
        # Log system info for debugging
        system_info = get_system_info()
        print(f"PDF Generation Debug Info: {json.dumps(system_info, indent=2)}")
        
        # Convert HTML to PDF using wkhtmltopdf with bottom margin for whitespace
        cmd = [
            'wkhtmltopdf',
            '--page-size', 'Letter',
            '--margin-top', '0',
            '--margin-right', '0',
            '--margin-bottom', '24',  # ~0.33in bottom whitespace
            '--margin-left', '0',
            '--print-media-type',
            '--zoom', '1.0',
            '--dpi', '96',
            '--disable-smart-shrinking',
            html_path,
            output_path
        ]
        
        print(f"Running command: {' '.join(cmd)}")
        result = run(cmd, capture_output=True, text=True)

        if result.returncode != 0:
            print(f"wkhtmltopdf stderr: {result.stderr}")
            return False, f"wkhtmltopdf failed: {result.stderr}"
        
        print(f"wkhtmltopdf stdout: {result.stdout}")
        os.unlink(html_path)
        return True, None
    except Exception as e:
        return False, str(e)

def main():
    if len(sys.argv) != 4:
        print("Usage: generate_resume.py <template_name> <user_data_json> <output_path>")
        sys.exit(1)

    template_name = sys.argv[1]
    try:
        user_data = json.loads(sys.argv[2])
    except json.JSONDecodeError as e:
        print(f"Failed to parse user data JSON: {e}")
        sys.exit(1)
    output_path = sys.argv[3]

    is_pdf = output_path.lower().endswith('.pdf')
    if is_pdf:
        success, error = generate_pdf_resume(template_name, user_data, output_path)
    else:
        success, error = generate_html_resume(template_name, user_data, output_path)
    if not success:
        print(f"Failed to generate resume: {error}")
        sys.exit(1)

    print(f"Successfully generated {'PDF' if is_pdf else 'HTML'} resume at {output_path}")

if __name__ == '__main__':
    main()