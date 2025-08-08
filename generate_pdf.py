#!/usr/bin/env python3
import sys
import json
import os
from weasyprint import HTML, CSS
from weasyprint.text.fonts import FontConfiguration

def generate_pdf(html_content, output_path):
    """Generate PDF from HTML content using WeasyPrint"""
    try:
        # Configure fonts
        font_config = FontConfiguration()
        
        # Create HTML object
        html = HTML(string=html_content)
        
        # Define CSS for PDF styling
        css_content = """
        @page {
            size: A4;
            margin: 0.5in;
        }
        body {
            font-family: Arial, sans-serif;
            font-size: 11pt;
            line-height: 1.15;
            color: #374151;
        }
        .preview {
            min-height: auto !important;
            box-shadow: none !important;
            border: none !important;
        }
        .preview * {
            page-break-inside: avoid;
        }
        .preview::after {
            display: none !important;
            content: none !important;
        }
        """
        
        css = CSS(string=css_content, font_config=font_config)
        
        # Generate PDF
        html.write_pdf(output_path, stylesheets=[css], font_config=font_config)
        
        print(f"PDF generated successfully: {output_path}")
        return True
        
    except Exception as e:
        print(f"Error generating PDF: {e}")
        return False

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python3 generate_pdf.py <html_content_file> <output_pdf_path>")
        sys.exit(1)
    
    html_file = sys.argv[1]
    output_path = sys.argv[2]
    
    # Read HTML content
    with open(html_file, 'r', encoding='utf-8') as f:
        html_content = f.read()
    
    # Generate PDF
    success = generate_pdf(html_content, output_path)
    sys.exit(0 if success else 1)
