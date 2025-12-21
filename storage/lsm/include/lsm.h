#ifndef LSM_ENGINE_C_API_H
#define LSM_ENGINE_C_API_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque handles for engine and options
typedef struct lsm_db_t lsm_db_t;
typedef struct lsm_options_t lsm_options_t;
typedef struct lsm_writebatch_t lsm_writebatch_t;
typedef struct lsm_iterator_t lsm_iterator_t;

// ======== Options ========
lsm_options_t* lsm_options_create();
void lsm_options_destroy(lsm_options_t* options);
void lsm_options_set_create_if_missing(lsm_options_t* options, uint8_t value);
// Add more options like compression, cache size, etc.

// ======== Database Operations ========
lsm_db_t* lsm_db_open(const lsm_options_t* options, const char* path, char** errptr);
void lsm_db_close(lsm_db_t* db);

void lsm_put(lsm_db_t* db, const char* key, size_t keylen, const char* val, size_t vallen, char** errptr);
// Returned value must be freed with lsm_free()
char* lsm_get(lsm_db_t* db, const char* key, size_t keylen, size_t* vallen, char** errptr);
void lsm_delete(lsm_db_t* db, const char* key, size_t keylen, char** errptr);

// ======== Write Batch for atomic writes ========
lsm_writebatch_t* lsm_writebatch_create();
void lsm_writebatch_destroy(lsm_writebatch_t* b);
void lsm_writebatch_put(lsm_writebatch_t* b, const char* key, size_t keylen, const char* val, size_t vallen);
void lsm_writebatch_delete(lsm_writebatch_t* b, const char* key, size_t keylen);
void lsm_write(lsm_db_t* db, lsm_writebatch_t* b, char** errptr);
void lsm_writebatch_clear(lsm_writebatch_t* b);

// ======== Iterator (Optional but Recommended) ========
lsm_iterator_t* lsm_iterator_create(lsm_db_t* db);
void lsm_iterator_destroy(lsm_iterator_t* iter);
uint8_t lsm_iterator_valid(const lsm_iterator_t* iter);
void lsm_iterator_seek_to_first(lsm_iterator_t* iter);
void lsm_iterator_seek(lsm_iterator_t* iter, const char* key, size_t keylen);
void lsm_iterator_next(lsm_iterator_t* iter);
const char* lsm_iterator_key(const lsm_iterator_t* iter, size_t* keylen);
const char* lsm_iterator_value(const lsm_iterator_t* iter, size_t* vallen);

// ======== Memory Management ========
void lsm_free(void* ptr);

#ifdef __cplusplus
} // extern "C"
#endif

#endif // LSM_ENGINE_C_API_H
