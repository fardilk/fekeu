package main

import (
"fmt"
"golang.org/x/crypto/bcrypt"
"os"
)

func main(){
pw := "useruser"
if len(os.Args) > 1 { pw = os.Args[1] }
h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
if err != nil { panic(err) }
fmt.Println(string(h))
}
