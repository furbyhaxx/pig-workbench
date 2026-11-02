int add(int a, int b) { int s = 0; for (int i = 0; i < b; i++) s += a; return s; }
int main(int argc, char **argv) { return add(argc, 3) > 5 ? 1 : 0; }
