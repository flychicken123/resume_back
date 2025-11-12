#!/usr/bin/env python3
import json
import os
import sys
from subprocess import run
import shutil
from pathlib import Path


def sanitize_text(value):
    """Remove lone surrogate code points that cannot be encoded to UTF-8."""
    if not isinstance(value, str):
        value = str(value)
    return "".join(ch for ch in value if not (0xD800 <= ord(ch) <= 0xDFFF))

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
    html_content = sanitize_text(html_content)
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
        preferred_engine = (user_data.get('engine') or '').lower()
        skip_chrome = preferred_engine in {'wkhtml', 'wkhtmltopdf', 'wkhtml-strict', 'none'}
        require_chrome = preferred_engine in {'chromium-strict', 'chromium_required'}
        chrome_result = None

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
        
        # Try headless-Chrome generation first for best fidelity when desired
        if not skip_chrome:
            chrome_result = _try_generate_pdf_with_chrome(html_path, output_path)
        else:
            print("Skipping Chromium engine per request; using wkhtmltopdf")

        if chrome_result:
            print("Chromium PDF generation succeeded")
        else:
            if chrome_result is False and require_chrome:
                return False, "Chromium engine requested but unavailable"
            if chrome_result is False and not skip_chrome:
                print("Chrome-based PDF generation not available; falling back to wkhtmltopdf")
            wk_result, wk_err = _generate_pdf_with_wkhtml(html_path, output_path)
            if not wk_result:
                return False, wk_err

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


def _file_uri(path: str) -> str:
    try:
        return Path(path).resolve().as_uri()
    except Exception:
        # Fallback simple URI builder
        abspath = os.path.abspath(path)
        if os.name == 'nt':
            return 'file:///' + abspath.replace('\\', '/')
        return 'file://' + abspath


def _try_generate_pdf_with_chrome(html_path: str, output_path: str) -> bool:
    """Attempt to generate the PDF with headless Chrome/Chromium.

    Prefers Playwright if available; otherwise tries a chromium binary from env.
    Returns True on success, False if chrome path isn't available; raises on hard errors.
    """
    # Option A: Playwright (preferred)
    try:
        from playwright.sync_api import sync_playwright  # type: ignore
        print("Using Playwright Chromium for PDF generation")
        file_url = _file_uri(html_path)
        with sync_playwright() as p:
            browser = p.chromium.launch()
            context = browser.new_context()
            page = context.new_page()
            page.goto(file_url, wait_until='load')
            page.emulate_media(media='print')
            # Prefer CSS @page size from HTML (Letter with margin 0 is already injected)
            page.pdf(
                path=output_path,
                print_background=True,
                margin={"top": "0", "right": "0", "bottom": "0", "left": "0"},
                prefer_css_page_size=True,
                scale=1.0,
            )
            browser.close()
        return True
    except ModuleNotFoundError:
        print("Playwright not installed; trying Chromium binary path")
    except Exception as e:
        print(f"Playwright path failed: {e}")
        # Try binary as fallback

    # Option B: Chromium/Chrome executable
    chromium_bin = _locate_chrome_binary() 
    file_url = _file_uri(html_path)
    # --headless=new preferred for modern Chromes
    cmd = [
        chromium_bin,
        '--headless=new',
        '--disable-gpu',
        '--no-sandbox',
        '--print-to-pdf-no-header',
        f'--print-to-pdf={os.path.abspath(output_path)}',
        '--disable-dev-shm-usage',
        file_url,
    ]
    try:
        print(f"Running chromium CLI: {' '.join(cmd)}")
        res = run(cmd, capture_output=True, text=True)
        if res.returncode == 0 and os.path.exists(output_path):
            return True
        print(f"Chromium CLI stderr: {res.stderr}")
        return False
    except FileNotFoundError:
        print("Chromium binary not found in PATH or env; skipping")
        return False
    except Exception as e:
        print(f"Chromium CLI path failed: {e}")
        return False


def _generate_pdf_with_wkhtml(html_path: str, output_path: str):
    """Generate PDF using wkhtmltopdf. Returns (success, error)."""
    zoom = os.getenv('WKHTMLTOPDF_ZOOM', '1.05')
    cmd = [
        'wkhtmltopdf',
        '--page-size', 'Letter',
        '--page-height', '11in',
        '--margin-top', '0',
        '--margin-right', '0',
        '--margin-bottom', '0',
        '--margin-left', '0',
        '--print-media-type',
        '--zoom', str(zoom),
        '--dpi', '96',
        '--disable-smart-shrinking',
        '--user-style-sheet', 'data:text/css,body{margin:0;padding:0;}*{box-sizing:border-box;}@page{margin:0;}',
        html_path,
        output_path
    ]
    print(f"Running wkhtmltopdf: {' '.join(cmd)}")
    result = run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"wkhtmltopdf stderr: {result.stderr}")
        return False, f"wkhtmltopdf failed: {result.stderr}"
    print(f"wkhtmltopdf stdout: {result.stdout}")
    return True, None

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

def _locate_chrome_binary() -> str:
    """Try to locate a Chrome/Chromium/Edge binary across platforms.

    Order of preference: env vars -> PATH -> common install paths.
    """
    # Env vars first
    env_candidates = [
        os.getenv('CHROMIUM_BIN'),
        os.getenv('CHROME_BIN'),
        os.getenv('GOOGLE_CHROME_BIN'),
    ]
    for c in env_candidates:
        if c and os.path.exists(c):
            print(f"Using chrome binary from env: {c}")
            return c

    # PATH
    path_candidates = [
        'chromium', 'chromium-browser', 'google-chrome', 'google-chrome-stable', 'chrome', 'msedge'
    ]
    for name in path_candidates:
        p = shutil.which(name)
        if p:
            print(f"Using chrome binary from PATH: {p}")
            return p

    # Common Windows locations
    if os.name == 'nt':
        program_files = os.environ.get('ProgramFiles', r'C:\\Program Files')
        program_files_x86 = os.environ.get('ProgramFiles(x86)', r'C:\\Program Files (x86)')
        local_app_data = os.environ.get('LOCALAPPDATA', os.path.expanduser(r'~\\AppData\\Local'))
        win_candidates = [
            os.path.join(program_files, 'Google', 'Chrome', 'Application', 'chrome.exe'),
            os.path.join(program_files_x86, 'Google', 'Chrome', 'Application', 'chrome.exe'),
            os.path.join(local_app_data, 'Google', 'Chrome', 'Application', 'chrome.exe'),
            os.path.join(program_files, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
            os.path.join(program_files_x86, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
        ]
        for p in win_candidates:
            if os.path.exists(p):
                print(f"Using chrome binary from common path: {p}")
                return p

    # macOS typical paths
    if sys.platform == 'darwin':
        mac_candidates = [
            '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
            '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
            '/Applications/Chromium.app/Contents/MacOS/Chromium',
        ]
        for p in mac_candidates:
            if os.path.exists(p):
                print(f"Using chrome binary from common path: {p}")
                return p

    # Linux common paths
    linux_candidates = [
        '/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/usr/bin/chromium', '/usr/bin/chromium-browser'
    ]
    for p in linux_candidates:
        if os.path.exists(p):
            print(f"Using chrome binary from common path: {p}")
            return p

    # Fallback
    return 'chromium'


if __name__ == '__main__':
    main()
