# Dummy package

This package only exists because of a limitation in the way the go runtime makes dependency information available
at runtime.

For the moment you can only get this information in a test if you are in a main package that defines a main function.

https://github.com/golang/go/issues/33976

Once its safe to upgrade the sentry dependency again, we can remove this package entirely.