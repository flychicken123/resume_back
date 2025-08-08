#!/usr/bin/env python3
"""
Resume Generator Script
Generates HTML and PDF resumes from template data
"""

import sys
import json
import os
from datetime import datetime
import subprocess

def generate_html_resume(template_name, user_data, output_path):
    """Generate HTML resume"""
    
    # Extract user data
    name = user_data.get('name', '')
    email = user_data.get('email', '')
    phone = user_data.get('phone', '')
    summary = user_data.get('summary', '')
    experience = user_data.get('experience', '')
    education = user_data.get('education', '')
    skills = user_data.get('skills', [])
    position = user_data.get('position', '')
    
    # Process experience into bullet points
    experience_lines = experience.split('\n') if experience else []
    experience_bullets = ''.join([f'<div class="achievement">• {line.strip()}</div>' for line in experience_lines if line.strip()])
    
    # Generate HTML content with reduced white space
    html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{name} - Resume</title>
    <style>
        body {{
            font-family: Arial, sans-serif;
            line-height: 1.4;
            margin: 0;
            padding: 15px;
            background-color: white;
        }}
        .header {{
            text-align: center;
            border-bottom: 3px solid #3498db;
            padding-bottom: 8px;
            margin-bottom: 15px;
        }}
        .name {{
            font-size: 24px;
            font-weight: bold;
            color: #2c3e50;
            margin-bottom: 4px;
        }}
        .contact {{
            color: #7f8c8d;
            font-size: 14px;
        }}
        .section {{
            margin-bottom: 15px;
        }}
        .section-title {{
            font-size: 18px;
            font-weight: bold;
            color: #3498db;
            border-bottom: 2px solid #3498db;
            padding-bottom: 4px;
            margin-bottom: 8px;
            text-transform: uppercase;
        }}
        .experience-item {{
            margin-bottom: 12px;
        }}
        .job-title {{
            font-weight: bold;
            color: #2c3e50;
        }}
        .company {{
            color: #7f8c8d;
            font-style: italic;
        }}
        .achievement {{
            margin-left: 20px;
            margin-bottom: 4px;
        }}
    </style>
</head>
<body>
    <div class="header">
        <div class="name">{name}</div>
        <div class="contact">{email} • {phone}</div>
    </div>
    
    <div class="section">
        <div class="section-title">Experience</div>
        <div class="experience-item">
            <div class="job-title">{position} Seattle, WA • Nov 2022 - Nov 2024</div>
            <div class="achievement">• Spearheaded the design, planning, construction, and maintenance of a new e-commerce logistics monitoring system, directly enhancing the reliability and observability of Stripe core operational metrics.</div>
            <div class="achievement">• Led backend development and integration of predictive AI models to forecast delivery risks and optimize fulfillment operations, resulting in increased platform reliability and scalability.</div>
            <div class="achievement">• Mentored junior engineers and created comprehensive documentation, fostering a knowledge-sharing and metrics-driven culture within the team.</div>
        </div>
        <div class="experience-item">
            <div class="job-title">Senior Software Engineer at Twillio Remote • May 2022 - Nov 2022</div>
            <div class="achievement">• Lead to developing a Customers Data Protection using Golang and Kafka, establishing a centralized system for data management.</div>
            <div class="achievement">• Design the De-identification Policy management framework to ensure data privacy compliance.</div>
        </div>
        <div class="experience-item">
            <div class="job-title">Senior Software Engineer at eBay Austin, Texas • Mar 2021 - Apr 2022</div>
            <div class="achievement">• Served in Risk Management department, developing tools for deploying mathematical models.</div>
            <div class="achievement">• Built queue distribution module and processed data with Spark and MapReduce.</div>
        </div>
        <div class="experience-item">
            <div class="job-title">Software Engineer at T-Mobile Austin, Texas • Aug 2020 - Mar 2021</div>
            <div class="achievement">• Collaborated with 3 engineers developing API for database migration and implemented 1 API per 1 day averagely.</div>
            <div class="achievement">• Assisted department director coaching and interviewing junior engineers.</div>
        </div>
        <div class="experience-item">
            <div class="job-title">Software Engineer at General Motor Austin, Texas • Jan 2015 - Aug 2020</div>
            <div class="achievement">• Led project to develop Java automation program on High Performance Computing Cloud.</div>
            <div class="achievement">• Converted Struts framework to Spring framework and designed/built a Rest API with Spring boot and Cassandra.</div>
        </div>
    </div>
    
    <div class="section">
        <div class="section-title">Education</div>
        <div class="experience-item">
            <div class="job-title">Bachelor of Science in Electrical Engineering Texas Tech University • 2014</div>
        </div>
    </div>
</body>
</html>"""
    
    # Write HTML file
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(html_content)
    
    print(f"HTML resume generated: {output_path}")

def generate_pdf_resume(template_name, user_data, output_path):
    """Generate PDF resume using wkhtmltopdf"""
    # Create HTML version first
    html_path = output_path.replace('.pdf', '.html')
    generate_html_resume(template_name, user_data, html_path)
    
    try:
        # Try to use wkhtmltopdf if available
        cmd = ['wkhtmltopdf', '--page-size', 'A4', '--margin-top', '0.5in', 
               '--margin-right', '0.5in', '--margin-bottom', '0.5in', 
               '--margin-left', '0.5in', html_path, output_path]
        
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        
        if result.returncode == 0:
            print(f"PDF resume generated: {output_path}")
            # Clean up the temporary HTML file
            os.remove(html_path)
        else:
            print(f"wkhtmltopdf failed: {result.stderr}")
            # Fallback: copy HTML content as PDF (for compatibility)
            with open(html_path, 'r', encoding='utf-8') as f:
                html_content = f.read()
            with open(output_path, 'w', encoding='utf-8') as f:
                f.write(html_content)
            print(f"Resume generated: {output_path} (HTML format as fallback)")
            
    except FileNotFoundError:
        print("wkhtmltopdf not found, using HTML fallback")
        # Fallback: copy HTML content as PDF
        with open(html_path, 'r', encoding='utf-8') as f:
            html_content = f.read()
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(html_content)
        print(f"Resume generated: {output_path} (HTML format as fallback)")
    except Exception as e:
        print(f"PDF generation error: {e}")
        # Fallback: copy HTML content as PDF
        with open(html_path, 'r', encoding='utf-8') as f:
            html_content = f.read()
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(html_content)
        print(f"Resume generated: {output_path} (HTML format as fallback)")

def main():
    if len(sys.argv) != 4:
        print("Usage: python generate_resume.py <template_name> <user_data_json> <output_path>")
        sys.exit(1)
    
    template_name = sys.argv[1]
    user_data_json = sys.argv[2]
    output_path = sys.argv[3]
    
    try:
        # Parse user data - handle both string and dict input
        if isinstance(user_data_json, str):
            user_data = json.loads(user_data_json)
        else:
            user_data = user_data_json
        
        # Determine output format based on file extension
        if output_path.endswith('.pdf'):
            generate_pdf_resume(template_name, user_data, output_path)
        else:
            generate_html_resume(template_name, user_data, output_path)
            
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"Error generating resume: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
