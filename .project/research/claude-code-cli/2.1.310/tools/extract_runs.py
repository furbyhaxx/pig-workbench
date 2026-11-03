import mmap, re, sys, os

path = "/tmp/claude.bun"
mincode, minstr = 160, 24

# printable run: 0x20-0x7e, plus tab/newline for code blocks
code_re = re.compile(rb"[\x20-\x7e\t\n]{%d,}" % mincode)
str_re  = re.compile(rb"[\x20-\x7e]{%d,}" % minstr)

with open(path, "rb") as f:
    mm = mmap.mmap(f.fileno(), 0, access=mmap.ACCESS_READ)
    with open("/tmp/cc_code.txt", "wb") as co, open("/tmp/cc_strings.txt", "wb") as so:
        for m in code_re.finditer(mm):
            co.write(b"@%d@" % (m.start() + 0x6e14000))
            co.write(m.group())
            co.write(b"\n")
        for m in str_re.finditer(mm):
            so.write(b"%d\t" % (m.start() + 0x6e14000))
            so.write(m.group())
            so.write(b"\n")
print("done")
