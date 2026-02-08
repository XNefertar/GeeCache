#include "logging/logging.h"
#include "logging/async_logging.h"
#include <unistd.h>

int main() {
    lsm::setupAsyncLogging("test_log.log", 1);
    LOG_INFO << "Hello world";
    LOG_WARN << "This is a warning";
    LOG_ERROR << "This is an error with number: " << 123;
    
    // Test formatting
    LOG_INFO << "Test bool: " << true;
    LOG_INFO << "Test double: " << 3.14159;
    
    sleep(2); // Wait for async log flush
    return 0;
}
