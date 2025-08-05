#!/usr/bin/env python3
import sys
import json
import os
from datetime import datetime
import subprocess
import tempfile

def generate_html_resume(user_data, template_name):
    """
    Generate HTML resume from user data with different templates
    """
    
    # Debug: Print received data
    print(f"DEBUG: Received user_data: {user_data}")
    print(f"DEBUG: Experience data: {user_data.get('experience')}")
    print(f"DEBUG: Education data: {user_data.get('education')}")
    print(f"DEBUG: Using template: {template_name}")
    
    # Get template styles
    styles = get_template_styles(template_name)
    print(f"DEBUG: Template styles length: {len(styles)}")
    print(f"DEBUG: Template styles preview: {styles[:200]}...")
    
    # Format experience data
    experience_html = ""
    if user_data.get("experience"):
        experiences = user_data["experience"].split('\n\n')
        for exp in experiences:
            if exp.strip():
                lines = exp.strip().split('\n')
                if len(lines) >= 1:
                    # First line is job title and company
                    job_info = lines[0].strip()
                    
                    # Create experience item
                    experience_html += f"""
    <div class="experience-item">
        <div class="institution-header">
            {job_info}
        </div>
"""
                    
                    # Add bullet points if they exist
                    if len(lines) > 1:
                        experience_html += """        <ul class="bullet-points">
"""
                        for line in lines[1:]:
                            cleaned_line = line.strip()
                            if cleaned_line and not cleaned_line.startswith('•'):
                                # Add bullet point if not already present
                                if cleaned_line:
                                    experience_html += f"            <li>{cleaned_line}</li>\n"
                        experience_html += """        </ul>
"""
                    
                    experience_html += """    </div>
"""
    
    # Format education data
    education_html = ""
    if user_data.get("education"):
        education_lines = user_data["education"].split('\n\n')
        for edu in education_lines:
            if edu.strip():
                # Filter out placeholder/instructional text
                cleaned_edu = edu.strip()
                # Remove common placeholder patterns
                placeholder_patterns = [
                    'e.g.,',
                    'e.g.',
                    'for example',
                    'example:',
                    'placeholder',
                    '[degree name]',
                    '[university name]',
                    '[graduation year]',
                    '[gpa]',
                    '[honors]',
                    '[your name]',
                    '[field]',
                    '[school]',
                    '[year]',
                    '[location]'
                ]
                
                # Check if education entry contains placeholder text
                is_placeholder = any(pattern.lower() in cleaned_edu.lower() for pattern in placeholder_patterns)
                
                # Temporarily disable filtering to debug
                if len(cleaned_edu) > 3:  # Only include substantial content
                    # Parse the education string to extract components
                    # Format: "Bachelor of Science in Electrical Engineering from Texas Tech University (2014)"
                    parts = cleaned_edu.split(' from ')
                    if len(parts) >= 2:
                        degree_part = parts[0]
                        school_part = parts[1]
                        
                        # Extract year from parentheses
                        year = ""
                        if '(' in school_part and ')' in school_part:
                            year_start = school_part.find('(')
                            year_end = school_part.find(')')
                            year = school_part[year_start+1:year_end]
                            school_part = school_part[:year_start].strip()
                        
                        education_html += f"""
    <div class="education-item">
        <div class="institution-header">
            {degree_part}
        </div>
        <div class="education-details">
            {school_part} • {year}
        </div>
    </div>
"""
                    else:
                        # Fallback to original format if parsing fails
                        education_html += f"""
    <div class="education-item">
        <div class="institution-header">
            <span class="bold">{cleaned_edu}</span>
        </div>
    </div>
"""
    
    # Format skills
    skills_html = ""
    if user_data.get("skills"):
        if isinstance(user_data["skills"], list):
            # Filter out placeholder skills
            filtered_skills = []
            placeholder_patterns = [
                'skill 1',
                'skill 2',
                'skill 3',
                'skill 4',
                'skill 5',
                'skill 6',
                'e.g.,',
                'e.g.',
                'for example',
                'example:',
                'placeholder',
                '[skill]',
                '[your skill]'
            ]
            
            for skill in user_data["skills"]:
                cleaned_skill = skill.strip()
                is_placeholder = any(pattern.lower() in cleaned_skill.lower() for pattern in placeholder_patterns)
                if not is_placeholder and len(cleaned_skill) > 1:
                    filtered_skills.append(cleaned_skill)
            
            if filtered_skills:
                skills_text = " • ".join(filtered_skills)
                skills_html = f"""
    <div class="additional-item">
        <span class="bold">Technical Skills:</span> {skills_text}
    </div>
"""
        else:
            # Handle string format skills
            skills_text = user_data["skills"]
            # Filter out placeholder text
            placeholder_patterns = [
                'skill 1',
                'skill 2',
                'skill 3',
                'skill 4',
                'skill 5',
                'skill 6',
                'e.g.,',
                'e.g.',
                'for example',
                'example:',
                'placeholder',
                '[skill]',
                '[your skill]'
            ]
            
            is_placeholder = any(pattern.lower() in skills_text.lower() for pattern in placeholder_patterns)
            if not is_placeholder and len(skills_text.strip()) > 3:
                skills_html = f"""
    <div class="additional-item">
        <span class="bold">Technical Skills:</span> {skills_text}
    </div>
"""
    
    # Filter summary for placeholder text
    summary_text = user_data.get("summary", "")
    if summary_text:
        placeholder_patterns = [
            'e.g.,',
            'e.g.',
            'for example',
            'example:',
            'placeholder',
            '[your field]',
            '[key achievements]',
            '[your name]',
            'write a brief professional summary',
            'optional: let ai help you',
            'describe your professional background',
            'quantify the team improvement',
            '20% increase in code quality',
            '10% reduction in bug reports'
        ]
        
        is_placeholder = any(pattern.lower() in summary_text.lower() for pattern in placeholder_patterns)
        if is_placeholder or len(summary_text.strip()) < 10:
            summary_text = ""  # Don't include placeholder summary
    
    # Get template-specific styles
    template_styles = get_template_styles(template_name)
    
    # Generate the HTML resume
    html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Resume - {user_data.get('name', 'Resume')}</title>
    <style>
        @page {{
            margin: 0.75in;
            size: letter;
        }}
        
        body {{
            font-family: Calibri, Arial, sans-serif;
            font-size: 11pt;
            line-height: 1.15;
            margin: 0;
            padding: 0;
            color: #000;
            background: white;
        }}
        
        .header {{
            text-align: center;
            margin-bottom: 20pt;
        }}
        
        .name {{
            font-size: 16pt;
            font-weight: bold;
            text-transform: uppercase;
            margin-bottom: 5pt;
        }}
        
        .contact-info {{
            font-size: 11pt;
            margin-bottom: 10pt;
        }}
        
        .section-header {{
            font-size: 11pt;
            font-weight: bold;
            text-transform: uppercase;
            margin-top: 15pt;
            margin-bottom: 8pt;
            border-bottom: 1px solid #000;
            padding-bottom: 2pt;
        }}
        
        .education-item, .experience-item, .project-item, .activity-item {{
            margin-bottom: 12pt;
        }}
        
        .institution-header {{
            display: flex;
            justify-content: space-between;
            font-weight: bold;
            margin-bottom: 2pt;
        }}
        
        .degree-info {{
            margin-bottom: 1pt;
        }}
        
        .job-title {{
            font-style: italic;
            margin-bottom: 1pt;
        }}
        
        .company-info {{
            margin-bottom: 3pt;
        }}
        
        .bullet-points {{
            margin-left: 0;
            padding-left: 15pt;
        }}
        
        .bullet-points li {{
            margin-bottom: 3pt;
            text-align: justify;
        }}
        
        .additional-section {{
            margin-top: 10pt;
        }}
        
        .additional-item {{
            margin-bottom: 5pt;
        }}
        
        .bold {{
            font-weight: bold;
        }}
        
        .italic {{
            font-style: italic;
        }}
        
        /* Print styles */
        @media print {{
            body {{
                font-size: 11pt;
                -webkit-print-color-adjust: exact;
                print-color-adjust: exact;
            }}
            .section-header {{
                page-break-after: avoid;
            }}
            .experience-item, .education-item, .project-item {{
                page-break-inside: avoid;
            }}
        }}
        
        {template_styles}
    </style>
</head>
<body>
    <div class="header">
        <div class="name">{user_data.get('name', '[FIRST LAST]')}</div>
        <div class="contact-info">{user_data.get('phone', '[Phone Number]')} | {user_data.get('email', '[email@domain.com]')}</div>
    </div>

    {f'<div class="section-header">SUMMARY</div><div class="additional-item">{summary_text}</div>' if summary_text else ""}

    {f'<div class="section-header">EXPERIENCE</div>{experience_html}' if experience_html else ""}

    {f'<div class="section-header">EDUCATION</div>{education_html}' if education_html else ""}

    {f'<div class="section-header">SKILLS</div><div class="additional-section">{skills_html}</div>' if skills_html else ""}
</body>
</html>"""
    
    return html_content

def get_template_styles(template_name):
    """Get template-specific CSS styles"""
    if template_name == "industry-manager":
        return """
        body {
            font-family: Georgia, serif;
            font-size: 9pt;
            line-height: 1.3;
        }
        
        .header {
            text-align: center;
            margin-bottom: 25pt;
        }
        
        .name {
            font-size: 16pt;
            font-weight: bold;
            color: #2c3e50;
            margin-bottom: 10pt;
        }
        
        .contact-info {
            font-size: 8pt;
            color: #7f8c8d;
            margin-bottom: 15pt;
        }
        
        .section-header {
            font-size: 11pt;
            font-weight: bold;
            color: #2c3e50;
            border-bottom: 2px solid #34495e;
            padding-bottom: 4pt;
            margin-bottom: 10pt;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        
        .institution-header {
            font-weight: bold;
            color: #2c3e50;
            font-size: 10pt;
        }
        
        .bullet-points li {
            font-size: 8pt;
            line-height: 1.4;
        }
        """
    elif template_name == "modern":
        return """
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            font-size: 9pt;
            line-height: 1.4;
        }
        
        .header {
            text-align: left;
            margin-bottom: 20pt;
            border-bottom: 3px solid #3498db;
            padding-bottom: 15pt;
        }
        
        .name {
            font-size: 18pt;
            font-weight: 600;
            color: #2c3e50;
            margin-bottom: 10pt;
        }
        
        .contact-info {
            font-size: 8pt;
            color: #7f8c8d;
            margin-bottom: 0;
        }
        
        .section-header {
            font-size: 11pt;
            font-weight: 600;
            color: #3498db;
            border-bottom: 1px solid #bdc3c7;
            padding-bottom: 3pt;
            margin-bottom: 8pt;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        
        .institution-header {
            font-weight: 600;
            color: #2c3e50;
            font-size: 10pt;
        }
        
        .bullet-points li {
            font-size: 9pt;
            line-height: 1.5;
        }
        
        .education-details {
            font-size: 8pt;
            color: #666;
            margin-top: 0.25rem;
        }
        """
    elif template_name == "temp1":
        return """
        body {
            font-family: Arial, sans-serif;
            font-size: 9pt;
            line-height: 1.3;
        }
        
        .header {
            text-align: center;
            margin-bottom: 20pt;
        }
        
        .name {
            font-size: 16pt;
            font-weight: bold;
            color: #1f2937;
            margin-bottom: 10pt;
        }
        
        .contact-info {
            font-size: 8pt;
            color: #6b7280;
            margin-bottom: 15pt;
        }
        
        .section-header {
            font-size: 11pt;
            font-weight: bold;
            color: #1f2937;
            border-bottom: 1px solid #000;
            padding-bottom: 4pt;
            margin-bottom: 10pt;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        
        .institution-header {
            font-weight: bold;
            color: #1f2937;
            font-size: 10pt;
        }
        
        .bullet-points li {
            font-size: 8pt;
            line-height: 1.4;
        }
        
        .education-details {
            font-size: 8pt;
            color: #666;
            margin-top: 0.25rem;
        }
        """
    else:  # Default template (temp1)
        return ""

def convert_html_to_pdf(html_content, output_path):
    """Convert HTML content to PDF using wkhtmltopdf"""
    try:
        # Check if wkhtmltopdf is available
        import platform
        if platform.system() == "Windows":
            # On Windows, try the common installation path
            wkhtmltopdf_paths = [
                "wkhtmltopdf",  # If it's in PATH
                r"C:\Program Files\wkhtmltopdf\bin\wkhtmltopdf.exe",
                r"C:\Program Files (x86)\wkhtmltopdf\bin\wkhtmltopdf.exe"
            ]
            
            wkhtmltopdf_cmd = None
            for path in wkhtmltopdf_paths:
                try:
                    subprocess.run([path, '--version'], capture_output=True, check=True)
                    wkhtmltopdf_cmd = path
                    print(f"✅ wkhtmltopdf found at: {path}")
                    break
                except (subprocess.CalledProcessError, FileNotFoundError):
                    continue
            
            if not wkhtmltopdf_cmd:
                print("❌ wkhtmltopdf not found. Please install wkhtmltopdf.")
                return False
        else:
            # On Linux/Docker, use the standard command
            try:
                subprocess.run(['wkhtmltopdf', '--version'], capture_output=True, check=True)
                wkhtmltopdf_cmd = 'wkhtmltopdf'
                print("✅ wkhtmltopdf is available")
            except (subprocess.CalledProcessError, FileNotFoundError):
                print("❌ wkhtmltopdf not found. Please install wkhtmltopdf.")
                return False
        
        # Create temporary HTML file
        with tempfile.NamedTemporaryFile(mode='w', suffix='.html', delete=False, encoding='utf-8') as temp_html:
            temp_html.write(html_content)
            temp_html_path = temp_html.name
        
        print(f"📄 Created temporary HTML file: {temp_html_path}")
        print(f"📄 HTML file size: {os.path.getsize(temp_html_path)} bytes")
        
        # Ensure output directory exists
        output_dir = os.path.dirname(output_path)
        if output_dir and not os.path.exists(output_dir):
            os.makedirs(output_dir)
            print(f"📁 Created output directory: {output_dir}")
        
        # Convert HTML to PDF using wkhtmltopdf with more robust options
        # Use xvfb-run in Docker environment, but direct wkhtmltopdf on Windows
        if platform.system() == "Windows":
            # Windows doesn't need xvfb-run
            cmd = [
                wkhtmltopdf_cmd,
                '--page-size', 'Letter',
                '--margin-top', '0.75in',
                '--margin-right', '0.75in',
                '--margin-bottom', '0.75in',
                '--margin-left', '0.75in',
                '--encoding', 'UTF-8',
                '--no-outline',
                '--disable-smart-shrinking',
                '--print-media-type',
                '--enable-local-file-access',
                '--quiet',
                temp_html_path,
                output_path
            ]
        else:
            # Linux/Docker environment
            cmd = [
                'xvfb-run', '--server-args="-screen 0 1024x768x24"',
                wkhtmltopdf_cmd,
                '--page-size', 'Letter',
                '--margin-top', '0.75in',
                '--margin-right', '0.75in',
                '--margin-bottom', '0.75in',
                '--margin-left', '0.75in',
                '--encoding', 'UTF-8',
                '--no-outline',
                '--disable-smart-shrinking',
                '--print-media-type',
                '--enable-local-file-access',
                '--quiet',
                temp_html_path,
                output_path
            ]
        
        print(f"🔄 Running command: {' '.join(cmd)}")
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        
        # Clean up temporary HTML file
        os.unlink(temp_html_path)
        
        if result.returncode != 0:
            print(f"❌ wkhtmltopdf failed with return code: {result.returncode}")
            print(f"❌ Error output: {result.stderr}")
            print(f"❌ Standard output: {result.stdout}")
            raise Exception(f"wkhtmltopdf failed: {result.stderr}")
        
        # Verify PDF was created
        if not os.path.exists(output_path):
            print(f"❌ PDF file was not created: {output_path}")
            return False
        
        pdf_size = os.path.getsize(output_path)
        print(f"✅ PDF conversion successful")
        print(f"✅ PDF file size: {pdf_size} bytes")
        
        # Check if PDF is valid (should be at least 1KB)
        if pdf_size < 1024:
            print(f"⚠️  Warning: PDF file is very small ({pdf_size} bytes), may be corrupted")
            return False
        
        return True
    except subprocess.TimeoutExpired:
        print("❌ PDF conversion timed out after 30 seconds")
        return False
    except Exception as e:
        print(f"❌ PDF conversion failed: {e}")
        return False

def generate_resume_from_template(template_name, user_data, output_path):
    """Generate PDF resume from user data"""
    
    print(f"Generating PDF resume for template: {template_name}")
    print(f"Output path: {output_path}")
    
    # Generate HTML content
    html_content = generate_html_resume(user_data, template_name)
    
    # Convert to PDF
    if output_path.endswith('.pdf'):
        print("Attempting PDF conversion...")
        success = convert_html_to_pdf(html_content, output_path)
        if success:
            print(f"✅ PDF resume saved to: {output_path}")
            # Verify file exists
            import os
            if os.path.exists(output_path):
                print(f"✅ File exists: {output_path}")
                print(f"✅ File size: {os.path.getsize(output_path)} bytes")
            else:
                print(f"❌ File does not exist: {output_path}")
        else:
            # Fallback to HTML if PDF conversion fails
            html_path = output_path.replace('.pdf', '.html')
            with open(html_path, 'w', encoding='utf-8') as f:
                f.write(html_content)
            print(f"⚠️  PDF conversion failed, saved HTML instead: {html_path}")
            print(f"⚠️  You may need to install wkhtmltopdf: sudo apt-get install wkhtmltopdf")
            # Update the output path to return HTML instead
            output_path = html_path
    else:
        # Save as HTML
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(html_content)
        print(f"HTML resume saved to: {output_path}")

def create_basic_template(template_name):
    """This function is kept for compatibility but not used for HTML generation"""
    return None

def test_wkhtmltopdf():
    """Test if wkhtmltopdf is working properly"""
    try:
        # Create a simple test HTML
        test_html = """<!DOCTYPE html>
<html>
<head>
    <title>Test PDF</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; }
    </style>
</head>
<body>
    <h1>Test PDF Generation</h1>
    <p>If you can see this, wkhtmltopdf is working!</p>
    <p>Generated at: """ + datetime.now().strftime("%Y-%m-%d %H:%M:%S") + """</p>
</body>
</html>"""
        
        # Create test PDF
        test_pdf = "test_output.pdf"
        success = convert_html_to_pdf(test_html, test_pdf)
        
        if success and os.path.exists(test_pdf):
            size = os.path.getsize(test_pdf)
            print(f"✅ Test PDF created successfully: {test_pdf} ({size} bytes)")
            # Clean up test file
            os.unlink(test_pdf)
            return True
        else:
            print("❌ Test PDF creation failed")
            return False
    except Exception as e:
        print(f"❌ Test failed: {e}")
        return False

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python generate_resume.py <template_name> <user_data_json> <output_path>")
        print("Or run with 'test' as first argument to test wkhtmltopdf")
        sys.exit(1)
    
    template_name = sys.argv[1]
    
    # Test mode
    if template_name == "test":
        print("🧪 Testing wkhtmltopdf installation...")
        test_wkhtmltopdf()
        sys.exit(0)
    
    # Normal mode - requires 4 arguments
    if len(sys.argv) != 4:
        print("Usage: python generate_resume.py <template_name> <user_data_json> <output_path>")
        sys.exit(1)
    
    user_data_json = sys.argv[2]
    output_path = sys.argv[3]
    
    try:
        user_data = json.loads(user_data_json)
        generate_resume_from_template(template_name, user_data, output_path)
    except Exception as e:
        print(f"Error generating resume: {e}")
        sys.exit(1) 