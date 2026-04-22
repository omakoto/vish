# Vish

Vish is a posix compliant shell written in gloang.


## Testing

 - @./e2e/run_tests.sh contains an end-to-end test suite that run the same test both on Vish
 and `bash --posix`.

## Before commit

Before committing any changes, always make sure to run @./e2e/run_tests.sh and @presubmit.sh.
