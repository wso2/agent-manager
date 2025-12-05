#!/bin/bash

# List of extensions to check
extensions=("go" "sh" "yaml" "md")  # You can modify this list as needed

# Build the find pattern from the extensions array
find_pattern=""
for ext in "${extensions[@]}"; do
    find_pattern+=" -name '*.$ext' -o"
done
# Remove the trailing -o
find_pattern="${find_pattern%-o}"

# Use find with the constructed pattern
find_command="find . -type f \( $find_pattern \)"
files=$(eval $find_command)

# Iterate over the found files
for file in $files; do
    # Check if the last character is not a newline and add one if it isn't
    if [[ $(tail -c1 "$file" | wc -l) -eq 0 ]]; then
        echo "Adding newline to: $file"
        echo >> "$file"
    fi
done

echo "Finished processing files."
