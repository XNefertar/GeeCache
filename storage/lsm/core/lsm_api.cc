#include "lsm.h"
#include "db.h"
#include <cstring>
#include <string>
#include <cstdlib>

// Helper to allocate error string
static void set_error(char** errptr, const std::string& msg) {
    if (errptr) {
        *errptr = strdup(msg.c_str());
    }
}

extern "C" {

    struct lsm_db_t {
        lsm::DB* rep;
    };

    struct lsm_options_t {
        bool create_if_missing;
    };

    lsm_options_t* lsm_options_create() {
        return new lsm_options_t;
    }

    void lsm_options_destroy(lsm_options_t* options) {
        delete options;
    }

    void lsm_options_set_create_if_missing(lsm_options_t* options, uint8_t value) {
        options->create_if_missing = (value != 0);
    }

    lsm_db_t* lsm_db_open(const lsm_options_t* options, const char* path, char** errptr) {
        try {
            auto db = new lsm::DB(path);
            auto wrapper = new lsm_db_t;
            wrapper->rep = db;
            if (errptr) *errptr = nullptr;
            return wrapper;
        } catch (const std::exception& e) {
            set_error(errptr, e.what());
            return nullptr;
        }
    }

    void lsm_db_close(lsm_db_t* db) {
        if (db) {
            delete db->rep;
            delete db;
        }
    }

    void lsm_put(lsm_db_t* db, const char* key, size_t keylen, const char* val, size_t vallen, char** errptr) {
        try {
            db->rep->Put(std::string(key, keylen), std::string(val, vallen));
            if (errptr) *errptr = nullptr;
        } catch (const std::exception& e) {
            set_error(errptr, e.what());
        }
    }

    char* lsm_get(lsm_db_t* db, const char* key, size_t keylen, size_t* vallen, char** errptr) {
        try {
            std::string value;
            bool found = db->rep->Get(std::string(key, keylen), &value);
            if (found) {
                char* result = (char*)malloc(value.size());
                memcpy(result, value.data(), value.size());
                *vallen = value.size();
                if (errptr) *errptr = nullptr;
                return result;
            } else {
                if (errptr) *errptr = nullptr;
                return nullptr;
            }
        } catch (const std::exception& e) {
            set_error(errptr, e.what());
            return nullptr;
        }
    }

    void lsm_delete(lsm_db_t* db, const char* key, size_t keylen, char** errptr) {
        try {
            db->rep->Delete(std::string(key, keylen));
            if (errptr) *errptr = nullptr;
        } catch (const std::exception& e) {
            set_error(errptr, e.what());
        }
    }

    void lsm_free(void* ptr) {
        free(ptr);
    }

// Stub implementations for WriteBatch and Iterator to satisfy linker if needed later
// For now, we just leave them unimplemented or simple stubs if referenced.
// But since we only use Put/Get/Delete in Go bridge, we are fine.

} // extern "C"
