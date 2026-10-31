package modules

// forkLimits is what this system can really take away from a fork's helper. Only Linux can
// take a network away from one program without an administrator's help.
const forkLimits = "On Windows it can't change this server's files or start other programs, but it keeps its network access, so it could send what it reads over the internet."
