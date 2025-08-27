# Debounce Feature

The debounce feature prevents arch-unit from running more than once within a specified time period for the same directory. This is particularly useful when arch-unit is integrated into file watchers, IDEs, or CI/CD pipelines where multiple rapid runs might occur.

## Usage

```bash
arch-unit check --debounce=30s [directory]
```

## Duration Format

The debounce parameter accepts any Go duration format:
- `30s` - 30 seconds
- `5m` - 5 minutes  
- `1h` - 1 hour
- `2h30m` - 2 hours and 30 minutes
- `500ms` - 500 milliseconds

## How It Works

1. **First Run**: When arch-unit runs with `--debounce` for the first time on a directory, it performs the normal check and records the timestamp
2. **Subsequent Runs**: If arch-unit is run again on the same directory within the debounce period, it skips the check and exits immediately
3. **Cache Location**: Timestamps are stored in `~/.cache/arch-unit/` using a hash of the directory path as the filename
4. **Per-Directory**: Each directory has its own debounce timer - checking different directories is not affected

## Examples

### Basic Debounce
```bash
# First run - performs check
$ arch-unit check --debounce=30s ./src
[INFO] Found 15 Go files and 3 Python files
✓ No architecture violations found!

# Second run within 30 seconds - skipped
$ arch-unit check --debounce=30s ./src
⏳ Skipping check for './src' (last run was within 30s)
```

### File Watcher Integration
```bash
# In a file watcher script
while inotifywait -e modify -r ./src; do
    arch-unit check --debounce=5s ./src
done
```

### IDE Integration
Many IDEs can be configured to run commands on file save. Use debounce to prevent excessive runs:

```bash
arch-unit check --debounce=2s $(dirname "$FILE")
```

### CI/CD Pipeline
For rapid commits or parallel builds:
```bash
arch-unit check --debounce=10s ./src || exit 1
```

## Output Modes

### Standard Output
Shows a skip message when debouncing:
```
⏳ Skipping check for '.' (last run was within 30s)
```

### JSON/CSV/Other Formats
No output when skipped to maintain valid format structure.

## Cache Management

### Cache Location
- Linux/macOS: `~/.cache/arch-unit/`
- Windows: `%LOCALAPPDATA%\arch-unit\`

### Cache Files
- Files are named using a SHA256 hash of the directory path
- Each file contains JSON with path, last run time, and directory flag
- Files are automatically created and updated as needed

### Manual Cache Cleanup
```bash
# Remove all cache files
rm -rf ~/.cache/arch-unit/

# Remove cache for specific directory
rm ~/.cache/arch-unit/$(echo -n "/full/path/to/dir" | sha256sum | cut -d' ' -f1).json
```

### Automatic Cleanup
The cache system includes automatic cleanup functionality (currently not exposed via CLI but available programmatically).

## Use Cases

1. **Development Workflow**: Prevent repeated checks when saving multiple files quickly
2. **File Watchers**: Avoid excessive runs when monitoring directory changes  
3. **CI/CD**: Skip redundant checks in rapid deployment scenarios
4. **IDE Integration**: Smooth experience without blocking the editor
5. **Automated Testing**: Prevent test suite overload from architecture checks

## Performance Impact

- Cache lookup is very fast (single file read)
- No impact when debounce is not used
- Minimal overhead when enabled
- Cache files are small (typically <200 bytes each)

## Troubleshooting

### Debounce Not Working
1. Check that the duration format is valid:
   ```bash
   arch-unit check --debounce=invalid  # Will show error
   ```

2. Verify cache directory permissions:
   ```bash
   ls -la ~/.cache/arch-unit/
   ```

3. Check for cache file creation:
   ```bash
   arch-unit check --debounce=30s .
   ls -la ~/.cache/arch-unit/
   ```

### Force Bypass Debounce
Simply run without the `--debounce` flag:
```bash
arch-unit check ./src  # Always runs
```

### Different Directories
Debounce is per-directory, so these won't interfere:
```bash
arch-unit check --debounce=30s ./src    # Independent
arch-unit check --debounce=30s ./tests  # Independent  
```

## Best Practices

1. **Choose Appropriate Duration**: 
   - File watchers: 1-5 seconds
   - IDE integration: 2-10 seconds
   - CI/CD: 10-30 seconds

2. **Consider Your Workflow**: Too short may not be effective, too long may feel unresponsive

3. **Test Your Setup**: Verify debounce works as expected in your environment

4. **Monitor Cache Growth**: In high-traffic scenarios, consider periodic cache cleanup

5. **Combine with Other Flags**: Debounce works with all other arch-unit options:
   ```bash
   arch-unit check --debounce=10s --json --fail-on-violation ./src
   ```