#!/usr/bin/env python3
import json
import os
import sys
from subprocess import run

def generate_html_resume(template_name, user_data, output_path):
    """Generate HTML resume from template and user data."""
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
    """Generate PDF resume using wkhtmltopdf."""
    # First generate HTML
    html_path = output_path.replace('.pdf', '.html')
    success, error = generate_html_resume(template_name, user_data, html_path)
    if not success:
        return False, error

    try:
        # Convert HTML to PDF using wkhtmltopdf
        result = run([
            'wkhtmltopdf',
            '--page-size', 'Letter',
            '--margin-top', '0',
            '--margin-right', '0',
            '--margin-bottom', '0',
            '--margin-left', '0',
            '--zoom', '0.98',
            '--dpi', '96',
            '--disable-smart-shrinking',
            html_path,
            output_path
        ], capture_output=True, text=True)
        
        if result.returncode != 0:
            return False, f"wkhtmltopdf failed: {result.stderr}"
            
        # Clean up temporary HTML file
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

    # Determine output type from extension
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