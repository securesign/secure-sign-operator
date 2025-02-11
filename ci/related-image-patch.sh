#!/bin/bash
#
# Usage: ./related-image-patch.sh inputfile
#
# This script reads an input file containing lines like:
#   VARIABLE_NAME=value
#
# It outputs a JSON object where each variable becomes an entry in
# the "env" array with its name and corresponding value.
#

# Check that exactly one argument is provided.
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 inputfile"
    exit 1
fi

inputfile="$1"

# Start outputting the JSON structure.
output='{"spec": {"config": {"env": ['

first_entry=1

# Process the file line by line.
while IFS= read -r line || [ -n "$line" ]; do
    # Skip empty lines or lines without an '=' character.
    if [ -z "$line" ] || [[ "$line" != *"="* ]]; then
        continue
    fi

    # Split the line into key and value.
    key="${line%%=*}"
    value="${line#*=}"

    # Print a comma before each JSON object except the first.
    if [ $first_entry -eq 0 ]; then
        output+=","
    fi

    # Append a newline to have each record on its own line.
    output+=$'\n'

    # Output the JSON object for this environment variable.
    output+="{\"name\": \"$key\", \"value\": \"$value\"}"

    first_entry=0
done < "$inputfile"

# Close the JSON structure.
output+=$'\n'
output+=']}}}'

echo "$output"
