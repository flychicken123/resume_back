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
    # Determine HTML source: either provided path or content
    provided_html_path = user_data.get('htmlPath')
    if provided_html_path and os.path.exists(provided_html_path):
        html_path = provided_html_path
    else:
        html_path = output_path.replace('.pdf', '.html')
        success, error = generate_html_resume(template_name, user_data, html_path)
        if not success:
            return False, error

    try:
        # Log system info for debugging
        system_info = get_system_info()
        print(f"PDF Generation Debug Info: {json.dumps(system_info, indent=2)}")
        
        # Log HTML content details for debugging
        try:
            with open(html_path, 'r', encoding='utf-8') as f:
                html_content = f.read()
                print(f"HTML Content Length: {len(html_content)} characters")
                print(f"HTML Content Preview (first 500 chars): {html_content[:500]}")
                # Check for specific CSS properties
                if '@page' in html_content:
                    print("Found @page CSS rule in HTML")
                if '.preview' in html_content:
                    print("Found .preview CSS class in HTML")
                if 'width:' in html_content:
                    print("Found width CSS property in HTML")
        except Exception as e:
            print(f"Error reading HTML for logging: {e}")
        
        # Convert HTML to PDF using wkhtmltopdf with balanced margins
        cmd = [
            'wkhtmltopdf',
            '--page-size', 'Letter',
            # Minimal top margin to reduce white space on first page
            '--margin-top', '2',   # ~0.03in (minimal top whitespace for first page)
            '--margin-right', '0',
            # Minimal bottom margin
            '--margin-bottom', '2',  # ~0.03in (minimal bottom whitespace)
            '--margin-left', '0',
            '--print-media-type',
            '--zoom', '1.0',
            '--dpi', '96',
            '--disable-smart-shrinking',
            # Add custom CSS for page break controls and Skills section
            '--user-style-sheet', 'data:text/css,.experience-item{page-break-inside:avoid!important;break-inside:avoid!important;orphans:3!important;widows:3!important;}.education-item{page-break-inside:avoid!important;break-inside:avoid!important;orphans:3!important;widows:3!important;}.preview .section-header{page-break-after:avoid!important;break-after:avoid!important;orphans:3!important;widows:3!important;}.preview .skills-section-header{page-break-before:always!important;break-before:page!important;page-break-after:avoid!important;break-after:avoid!important;margin-top:0!important;}.preview .skills-content{page-break-before:avoid!important;page-break-inside:avoid!important;break-inside:avoid!important;orphans:3!important;widows:3!important;}',
            html_path,
            output_path
        ]
        
        print(f"Running command: {' '.join(cmd)}")
        result = run(cmd, capture_output=True, text=True)

        if result.returncode != 0:
            print(f"wkhtmltopdf stderr: {result.stderr}")
            return False, f"wkhtmltopdf failed: {result.stderr}"
        
        print(f"wkhtmltopdf stdout: {result.stdout}")
        
        # Log PDF file details
        if os.path.exists(output_path):
            pdf_size = os.path.getsize(output_path)
            print(f"Generated PDF size: {pdf_size} bytes")
        else:
            print("Warning: PDF file was not created")
        
        # Do not delete html if it was uploaded
        try:
            if not provided_html_path and os.path.exists(html_path):
                os.unlink(html_path)
        except Exception:
            pass
        return True, None
    except Exception as e:
        return False, str(e)

def main():
    if len(sys.argv) != 4:
        print("Usage: generate_resume.py <template_name> <user_data_json|'-'> <output_path>")
        sys.exit(1)

    template_name = sys.argv[1]
    # Support large payloads via stdin: pass '-' as the second arg
    if sys.argv[2] == '-':
        try:
            user_data = json.load(sys.stdin)
        except Exception as e:
            print(f"Failed to read user data JSON from stdin: {e}")
            sys.exit(1)
    else:
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