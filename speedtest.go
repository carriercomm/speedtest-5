// Usenet Speedtest
// Try to accurately measure usenet speed.
// tommy@chiparus.org
// This program is in the public domain.
//
// Use like:
// ./speedtest -server your.server.name:119 -user user@name -pass secret -arts 1000 -conns 10
//
// Example Output:
// Read 91537632 bytes in 72.49 seconds. 10.10 mbit.

package main

import (
	"code.google.com/p/chiparus-nntp-go/nntp"
	"flag"
	"fmt"
	"io/ioutil"
	"runtime"
	"sync"
	"time"
)

var user = flag.String("user", "", "Username for server.")
var pass = flag.String("pass", "", "Password for server.")
var server = flag.String("server", "", "NNTP hostname:port.")
var verbose = flag.Bool("v", false, "Be verbose.")
var conns = flag.Int("conns", 1, "Nr of connections.")
var arts = flag.Int64("arts", 100, "Nr of articles.")
var group = flag.String("group", "alt.binaries.boneless", "Newsgroup to use.")

var wg sync.WaitGroup

type Connection struct {
	nntp   *nntp.Conn
	closer chan bool
}

func (c *Connection) Connect(q chan string) (err error) {
	c.nntp, err = nntp.Dial("tcp", *server)
	if err != nil {
		panic(err)
	}

	msg := c.nntp.Msg()
	if msg[0] != '2' {
		panic(msg)
	}

	if *user != "" && *pass != "" {
		c.nntp.Authenticate(*user, *pass)
	}

	err = c.nntp.ModeReader()
	if err != nil {
		panic(err)
	}

	c.closer = make(chan bool)
	go c.readQueue(q)

	return nil
}

func (c *Connection) Close() {
	c.closer <- true
}

func (c *Connection) readArt(msgid string) {
	artr, err := c.nntp.ArticleText(msgid)
	if err != nil {
		fmt.Printf("Article: %s %v\n", msgid, err)
	} else {
		art, _ := ioutil.ReadAll(artr)
		lenart := len(art)
		totsize += lenart
		if *verbose {
			fmt.Printf("%p Retrieving article: %s (%d)\n", c, msgid, lenart)
		}
	}
	wg.Done()
}

func (c *Connection) readQueue(q chan string) {
	for {
		select {
		case <-c.closer:
			c.nntp.Quit()
			return
		case msgid := <-q:
			c.readArt(msgid)
		}
	}
}

var totsize = 0

func main() {
	flag.Parse()
	fmt.Printf("Retrieving data for %s..\n", *server)
	runtime.GOMAXPROCS(8)

	n, err := nntp.Dial("tcp", *server)
	if err != nil {
		panic(err)
	}

	if *user != "" && *pass != "" {
		n.Authenticate(*user, *pass)
	}
	n.ModeReader()

	_, lo, hi, err := n.Group(*group)
	if err != nil {
		panic(err)
	}

	// Start somewhere in the middle.
	lo = lo + (hi-lo)/2
	hi = lo + *arts

	over, err := n.Over(fmt.Sprintf("%d-%d", lo, hi))
	if err != nil {
		panic(err)
	}

	n.Quit()

	fmt.Printf("Starting connections..\n")

	var pool []*Connection
	var queue = make(chan string)

	for i := 0; i < *conns; i++ {
		con := &Connection{}
		con.Connect(queue)
		pool = append(pool, con)
	}

	t0 := time.Now()
	for _, ov := range over {
		queue <- ov.MessageId
		wg.Add(1)
	}

	wg.Wait()

	// Totals
	numsec := float64(time.Now().Sub(t0)) / float64(time.Second)
	size := float64(totsize) * 8.0
	fmt.Printf("Read %d bytes in %.2f seconds. %.2f mbit.\n", totsize, numsec, (size/numsec)/1000/1000)
}
