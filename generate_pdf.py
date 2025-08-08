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
        
        # Define CSS for PDF styling - more comprehensive
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
            margin: 0;
            padding: 0;
        }
        .preview {
            min-height: auto !important;
            box-shadow: none !important;
            border: none !important;
            padding: 0.5in;
        }
        .preview * {
            page-break-inside: avoid;
        }
        .preview::after {
            display: none !important;
            content: none !important;
        }
        .header {
            text-align: center;
            margin-bottom: 1rem;
        }
        .name {
            font-size: 18pt;
            font-weight: bold;
            color: #1f2937;
            margin-bottom: 0.5rem;
        }
        .contact-info {
            font-size: 10pt;
            color: #6b7280;
        }
        .section-header {
            font-size: 12pt;
            font-weight: bold;
            color: #1f2937;
            border-bottom: 1px solid #000;
            margin-top: 1rem;
            margin-bottom: 0.5rem;
            padding-bottom: 0.25rem;
        }
        .experience-item, .education-item {
            margin-bottom: 1rem;
        }
        .institution-header {
            font-size: 11pt;
            font-weight: bold;
            color: #374151;
            margin-bottom: 0.25rem;
        }
        .education-details {
            font-size: 10pt;
            color: #6b7280;
            margin-bottom: 0.5rem;
        }
        .bullet-points {
            margin: 0.5rem 0;
            padding-left: 1rem;
        }
        .bullet-points li {
            margin-bottom: 0.25rem;
            line-height: 1.2;
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
