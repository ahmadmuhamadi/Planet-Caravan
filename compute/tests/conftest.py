import sys
from pathlib import Path

# The service is a flat app (not an installed package); make `import main` work
# regardless of the directory pytest is launched from.
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
