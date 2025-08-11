#!/usr/bin/env python3
import sys
import os
import json
import re
import subprocess

# Optional imports guarded; we won't crash if a format lib is missing
try:
    # Try different import paths for pdfminer
    try:
        from pdfminer.high_level import extract_text as pdf_extract_text
    except ImportError:
        try:
            from pdfminer.pdfinterp import PDFResourceManager, PDFPageInterpreter
            from pdfminer.converter import TextConverter
            from pdfminer.layout import LAParams
            from pdfminer.pdfpage import PDFPage
            from io import StringIO
            
            def pdf_extract_text(path):
                resource_manager = PDFResourceManager()
                string_io = StringIO()
                converter = TextConverter(resource_manager, string_io, laparams=LAParams())
                with open(path, 'rb') as file:
                    interpreter = PDFPageInterpreter(resource_manager, converter)
                    for page in PDFPage.create_pages(file):
                        interpreter.process_page(page)
                text = string_io.getvalue()
                converter.close()
                string_io.close()
                return text
        except ImportError:
            pdf_extract_text = None
except Exception as e:
    print(f"PDF extraction setup failed: {e}", file=sys.stderr)
    pdf_extract_text = None

# Try PyMuPDF (fitz) as another PDF extraction method
try:
    import fitz  # PyMuPDF
    def fitz_extract_text(path):
        try:
            doc = fitz.open(path)
            text = ""
            for page in doc:
                text += page.get_text()
            doc.close()
            return text
        except Exception as e:
            print(f"PyMuPDF extraction failed: {e}", file=sys.stderr)
            return ""
except ImportError:
    fitz_extract_text = None

# Try poppler-utils as a system command fallback
def poppler_extract_text(path):
    try:
        result = subprocess.run(['pdftotext', path, '-'], 
                              capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            return result.stdout
        else:
            print(f"Poppler extraction failed: {result.stderr}", file=sys.stderr)
            return ""
    except Exception as e:
        print(f"Poppler command failed: {e}", file=sys.stderr)
        return ""

try:
    import docx
except Exception as e:
    print(f"DOCX import failed: {e}", file=sys.stderr)
    docx = None

EMAIL_RE = re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}")
PHONE_RE = re.compile(r"(\+?\d[\d\-\s\(\)]{7,}\d)")

SECTION_KEYS = [
    'experience', 'work experience', 'professional experience',
    'education', 'skills', 'summary', 'objective', 'projects'
]


def read_txt(path: str) -> str:
    with open(path, 'r', encoding='utf-8', errors='ignore') as f:
        return f.read()


def read_pdf(path: str) -> str:
    """Try multiple PDF extraction methods in sequence"""
    methods = []
    
    if pdf_extract_text is not None:
        methods.append(("pdfminer", pdf_extract_text))
    if fitz_extract_text is not None:
        methods.append(("PyMuPDF", fitz_extract_text))
    methods.append(("poppler", poppler_extract_text))
    
    for method_name, method_func in methods:
        try:
            print(f"Trying {method_name} extraction...", file=sys.stderr)
            text = method_func(path)
            if text and text.strip():
                print(f"Successfully extracted {len(text)} characters using {method_name}", file=sys.stderr)
                return text
            else:
                print(f"{method_name} returned empty text", file=sys.stderr)
        except Exception as e:
            print(f"{method_name} extraction failed: {e}", file=sys.stderr)
    
    print("All PDF extraction methods failed", file=sys.stderr)
    return ""


def read_docx(path: str) -> str:
    if docx is None:
        return ''
    try:
        d = docx.Document(path)
        return "\n".join(p.text for p in d.paragraphs)
    except Exception:
        return ''


def split_sections(text: str):
    # naive section split by header lines
    lines = [l.strip() for l in text.splitlines()]
    sections = {}
    current_key = 'summary'
    sections[current_key] = []
    for line in lines:
        lower = line.lower()
        if any(k in lower and len(line) < 60 for k in SECTION_KEYS):
            # start new section
            if lower.startswith('education'):
                current_key = 'education'
            elif 'experience' in lower:
                current_key = 'experience'
            elif 'skill' in lower:
                current_key = 'skills'
            elif 'project' in lower:
                current_key = 'projects'
            elif 'summary' in lower or 'objective' in lower:
                current_key = 'summary'
            else:
                current_key = line.lower()
            sections.setdefault(current_key, [])
            continue
        sections.setdefault(current_key, []).append(line)

    # join
    joined = {k: "\n".join(v).strip() for k, v in sections.items() if any(x for x in v)}
    return joined


def extract_basics(text: str):
    email = EMAIL_RE.search(text)
    phone = PHONE_RE.search(text)
    return {
        'email': email.group(0) if email else '',
        'phone': phone.group(0) if phone else '',
    }


def main():
    if len(sys.argv) != 2:
        print(json.dumps({'error': 'usage: parse_resume.py <file>'}), file=sys.stderr)
        sys.exit(1)

    path = sys.argv[1]
    ext = os.path.splitext(path)[1].lower()

    print(f"Processing file: {path} with extension: {ext}", file=sys.stderr)

    if ext == '.pdf':
        if pdf_extract_text is None:
            print("PDF extraction not available", file=sys.stderr)
            text = ''
        else:
            try:
                text = read_pdf(path)
                print(f"Extracted {len(text)} characters from PDF", file=sys.stderr)
            except Exception as e:
                print(f"PDF extraction failed: {e}", file=sys.stderr)
                text = ''
    elif ext in ('.docx', '.doc'):
        if docx is None:
            print("DOCX extraction not available", file=sys.stderr)
            text = ''
        else:
            try:
                text = read_docx(path)
                print(f"Extracted {len(text)} characters from DOCX", file=sys.stderr)
            except Exception as e:
                print(f"DOCX extraction failed: {e}", file=sys.stderr)
                text = ''
    else:
        try:
            text = read_txt(path)
            print(f"Extracted {len(text)} characters from text file", file=sys.stderr)
        except Exception as e:
            print(f"Text file reading failed: {e}", file=sys.stderr)
            text = ''

    if not text.strip():
        print("No text extracted from file", file=sys.stderr)
        out = {
            'raw_text': '',
            'email': '',
            'phone': '',
            'sections': {},
            'error': 'No text could be extracted from the file'
        }
    else:
        basics = extract_basics(text)
        sections = split_sections(text)
        
        print(f"Found email: {basics.get('email', 'None')}", file=sys.stderr)
        print(f"Found phone: {basics.get('phone', 'None')}", file=sys.stderr)
        print(f"Found sections: {list(sections.keys())}", file=sys.stderr)

        out = {
            'raw_text': text,
            'email': basics.get('email', ''),
            'phone': basics.get('phone', ''),
            'sections': sections,
        }
    
    # Only print JSON to stdout, everything else to stderr
    print(json.dumps(out, ensure_ascii=False), file=sys.stdout)

if __name__ == '__main__':
    main()


