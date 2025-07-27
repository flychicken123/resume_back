import sys
import json
try:
    from pyresparser import ResumeParser
    if len(sys.argv) < 2:
        print(json.dumps({"error": "No file provided"}))
        sys.exit(1)
    file_path = sys.argv[1]
    data = ResumeParser(file_path).get_extracted_data()
    print(json.dumps(data))
except Exception as e:
    print(json.dumps({"error": str(e)}))
    sys.exit(1)

if __name__ == "__main__":
    main()