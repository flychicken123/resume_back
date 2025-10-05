from pathlib import Path
path = Path("main.go")
text = path.read_text()
if '"context"' not in text.split('import (',1)[1]:
    text = text.replace('import (\n\t"fmt"', 'import (\n\t"context"\n\t"fmt"', 1)
    path.write_text(text)
