#!/usr/bin/env python3

import sys
import re
import os
from pathlib import Path

# This script checks that error messages in fmt.Errorf calls start with a
# specific wording to ensure consistency. It outputs results in a format
# that GitHub Actions can use to annotate code.

REQUIRED_WORDING = "failed to"

# We pre-compile the regex for performance reasons, as it will be used
# multiple times in a loop. The pattern is designed to find `fmt.Errorf`
# calls and capture the string literal passed to them.
ERROR_PATTERN = re.compile(r'fmt\.Errorf\("([^"]+)"')

def check_file(filepath: Path) -> list[dict]:
    """
    Checks a single file for fmt.Errorf message violations using regex.

    Args:
        filepath: The path to the Go file to check, as a Path object.

    Returns:
        A list of dictionaries, where each dictionary contains details of an error.

    Raises:
        FileNotFoundError: If the file does not exist.
        Exception: For other file processing errors.
    """
    invalid_lines = []
    with filepath.open('r', encoding='utf-8') as f:
        for i, line in enumerate(f, 1):
            match = ERROR_PATTERN.search(line)
            if match:
                error_message = match.group(1)
                if not error_message.startswith(REQUIRED_WORDING):
                    # GitHub annotations use 1-based indexing for columns, so we add 1.
                    start_col = match.start(1) + 1
                    # The end of the regex match is exclusive, which aligns with how
                    # `endColumn` is interpreted by GitHub Actions.
                    end_col = match.end(1)
                    invalid_lines.append({
                        'line_num': i,
                        'col_num': start_col,
                        'end_col_num': end_col,
                        'error_message': error_message,
                    })
    return invalid_lines

def main():
    """
    Main function to process command-line arguments and run checks.
    Outputs errors in a format suitable for GitHub Actions annotations.
    """
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <file1.go> <file2.go> ...")
        sys.exit(1)

    exit_code = 0
    # Using Path objects makes file path manipulation more robust and
    # platform-agnostic compared to using raw strings.
    filepaths = [Path(p) for p in sys.argv[1:]]

    total_errors = 0
    for path in filepaths:
        try:
            errors = check_file(path)
            if errors:
                exit_code = 1
                total_errors += len(errors)
                for error in errors:
                    line_num = error['line_num']
                    col_num = error['col_num']
                    end_col_num = error['end_col_num']
                    error_message = error['error_message']

                    # Format for GitHub Actions annotations:
                    # ::error title=<title>,file=<file>,line=<line>,col=<col>,endLine=<endLine>,endColumn=<endColumn>::<message>
                    title = "Incorrect error message format"
                    message = f'Error message must start with "{REQUIRED_WORDING}". Found: "{error_message}"'
                    print(f"::error title={title},file={path},line={line_num},col={col_num},endLine={line_num},endColumn={end_col_num}::{message}")

        except FileNotFoundError:
            exit_code = 1
            total_errors += 1
            # We can't provide line/col for a file that doesn't exist, so we
            # create a file-level annotation instead.
            print(f"::error file={path}::File not found.")
        except Exception as e:
            exit_code = 1
            total_errors += 1
            print(f"::error file={path}::Error processing file: {e}")

    if total_errors > 0:
        # When running in GitHub Actions, a job summary provides a high-level
        # overview of the linting results on the workflow run's summary page.
        summary_file_path = os.getenv("GITHUB_STEP_SUMMARY")
        if summary_file_path:
            with open(summary_file_path, "a") as f:
                f.write(f"### Go Error Message Linting\n\n")
                f.write(f"Found {total_errors} formatting error(s).\n")
        else:
            # For local execution, a simple summary is printed to standard output
            # to inform the user of the outcome.
            print(f"\nFound {total_errors} formatting error(s).")

    sys.exit(exit_code)

if __name__ == "__main__":
    main()
