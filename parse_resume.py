#!/usr/bin/env python3
import sys
import os
import json
import re

# Optional imports guarded; we won't crash if a format lib is missing
try:
    from pdfminer.high_level import extract_text as pdf_extract_text
except Exception:
    pdf_extract_text = None

try:
    import docx
except Exception:
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
    if pdf_extract_text is None:
        return ''
    try:
        return pdf_extract_text(path) or ''
    except Exception:
        return ''


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
        print(json.dumps({'error': 'usage: parse_resume.py <file>'}))
        sys.exit(1)

    path = sys.argv[1]
    ext = os.path.splitext(path)[1].lower()

    if ext == '.pdf':
        text = read_pdf(path)
    elif ext in ('.docx', '.doc'):
        text = read_docx(path)
    else:
        text = read_txt(path)

    basics = extract_basics(text)
    sections = split_sections(text)

    out = {
        'raw_text': text,
        'email': basics.get('email', ''),
        'phone': basics.get('phone', ''),
        'sections': sections,
    }
    print(json.dumps(out, ensure_ascii=False))

if __name__ == '__main__':
    main()

