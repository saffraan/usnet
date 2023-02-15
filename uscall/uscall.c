#include <stdio.h>
#include "uscall.h"
#include <ff_api.h>
#include <sys/ioctl.h>

int sys_ioctl_non_bio(int fd, int on){
    return ioctl(fd, FIONBIO,  &on);
}

int ff_ioctl_non_bio(int fd, int on){
    return ff_ioctl(fd, FIONBIO,  &on);
}

ssize_t ff_read_slice(int fd, slice* output){
    ssize_t nread = ff_read(fd, output->ptr + output->len, output->cap - output->len);
    if (nread > 0) output->len += nread;
    return nread;
}

ssize_t ff_write_slice(int fd, slice*  input){
    ssize_t nwrite = ff_write(fd, input->ptr, input->len);
    if (nwrite > 0) input->len -= nwrite;
    return nwrite;
}

ssize_t ff_read_cslice(int fd, slice* output){
    return  ff_read(fd, output->ptr,  output->len);
}

ssize_t ff_write_cslice(int fd, slice*  input){
    return  ff_write(fd, input->ptr, input->len);
}

ssize_t sys_read_cslice(int fd, slice* output){
    return  read(fd, output->ptr,  output->len);
}

ssize_t sys_write_cslice(int fd, slice*  input){
      return  write(fd, input->ptr, input->len);
}

slice* slice_alloc( uint32_t cap){
    slice* s = (slice *)malloc(sizeof(slice));
    s->cap, s->len = cap, 0;
    s->ptr = (char *)malloc(cap);
    memset((void*)s->ptr, 0, cap);
    return s;
}

int slice_alloc_1(slice* s, uint32_t cap){
    s->ptr = (slice *)malloc(cap);
    if (s->ptr == NULL) {
        return -1;
    }
    s->cap = cap;
    return cap;
}

void slice_free(slice* s){
    if (s != NULL) {
        if (s->ptr != NULL){
            free(s->ptr);
            s->ptr = NULL;
        }
        free(s);
        s= NULL;
    }
}

void slice_free_1(slice* s) {
     if (s != NULL && s->ptr != NULL){
            free(s->ptr);
            s->ptr = NULL;
            s->len, s->cap = 0, 0;
    }
}

void slice_clean(slice *s) {
    if (s!=NULL && s->ptr != NULL) {
        memset(s->ptr, 0, s->cap);
         s->len = 0;
        return;
    }
}

int slice_child(const slice* parent, slice *child,  uint32_t pos,  uint32_t len){
    if (parent->cap < pos + len){
        return -1;
    }

    child->len, child->cap = len, parent->cap - pos;
    child->ptr = parent->ptr + pos;
    return 0;
}

int sys_set_reuse_port(int fd) {
    int yes = 1;
    /* Make sure connection-intensive things like the redis benckmark
     * will be able to close/open sockets a zillion of times */
    return setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &yes, sizeof(yes));
}