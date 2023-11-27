package gol

import (
	"fmt"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	key        <-chan rune
}

func workers(p Params, world [][]byte, result chan<- [][]byte, start, end int) {
	worldPiece := nextState(p, world, start, end)
	result <- worldPiece
	close(result)
}

func copySlice(src [][]byte) [][]byte {
	dst := make([][]byte, len(src))
	for i := range src {
		dst[i] = make([]byte, len(src[i]))
		copy(dst[i], src[i])
	}
	return dst
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// The ioInput is just a const for operation
	// It determines the operation to do
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%vx%v", p.ImageHeight, p.ImageWidth)

	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	// Initialize the state
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
			if world[y][x] == 255 {
				c.events <- CellFlipped{0, util.Cell{X: x, Y: y}}
			}
		}
	}

	turnCount := 0
	turn := 0
	var cellCount int
	pasued := false
	var mutex sync.Mutex
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			mutex.Lock()
			if !pasued {
				c.events <- AliveCellsCount{turnCount, cellCount}
				mutex.Unlock()
			} else {
				mutex.Unlock()
			}
		}
	}()

	resume := make(chan bool)
	quit := make(chan bool)
	go func() {
		for {
			select {
			case key := <-c.key:
				switch key {
				case 's':
					c.ioCommand <- ioOutput
					mutex.Lock()
					c.ioFilename <- fmt.Sprintf("%vx%vx%v", p.ImageHeight, p.ImageWidth, turnCount)
					for y := 0; y < p.ImageHeight; y++ {
						for x := 0; x < p.ImageWidth; x++ {
							c.ioOutput <- world[y][x]
						}
					}
					mutex.Unlock()
				case 'q':
					c.ioCommand <- ioOutput
					mutex.Lock()
					c.ioFilename <- fmt.Sprintf("%vx%vx%v", p.ImageHeight, p.ImageWidth, turnCount)
					for y := 0; y < p.ImageHeight; y++ {
						for x := 0; x < p.ImageWidth; x++ {
							c.ioOutput <- world[y][x]
						}
					}
					c.events <- FinalTurnComplete{turn, calculateAliveCells(p, world)}
					mutex.Unlock()
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					c.events <- StateChange{turn, Quitting}
					quit <- true
				case 'p':
					mutex.Lock()
					pasued = !pasued
					turnNow := turnCount
					mutex.Unlock()
					if pasued {
						c.events <- StateChange{turnNow, Paused}
					} else {
						fmt.Println("Continuing")
						c.events <- StateChange{turnNow, Executing}
						resume <- true
					}
				}
			case <-quit:
				return
			}
		}
	}()
	// TODO: Execute all turns of the Game of Life.
	for ; turn < p.Turns; turn++ {
		mutex.Lock()
		if pasued {
			mutex.Unlock()
			<-resume
		} else {
			mutex.Unlock()
		}
		if p.Threads == 1 {
			world = nextState(p, world, 0, p.ImageHeight)
		} else {
			newSize := p.ImageHeight / p.Threads
			result := make([]chan [][]byte, p.Threads)

			for i := range result {
				result[i] = make(chan [][]byte)
			}

			for i := 0; i < p.Threads; i++ {
				start := i * newSize
				end := start + newSize
				if i == p.Threads-1 {
					end = p.ImageHeight
				}
				worldCopy := copySlice(world)
				go workers(p, worldCopy, result[i], start, end)
			}

			for i := 0; i < p.Threads; i++ {
				result := <-result[i]
				start := i * newSize
				// This ensures that we are copying the worker result to the correct place in the world.
				for j := start; j < start+len(result); j++ {
					for k := 0; k < p.ImageWidth; k++ {
						if result[j-start][k] != world[j][k] {
							c.events <- CellFlipped{turn + 1, util.Cell{X: k, Y: j}}
						}
					}
					mutex.Lock()
					copy(world[j], result[j-start])
					mutex.Unlock()
				}
			}
			mutex.Lock()
			turnCount++
			cellCount = len(calculateAliveCells(p, world))
			mutex.Unlock()
		}
		c.events <- TurnComplete{turn}
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, calculateAliveCells(p, world)}

	// Output
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%vx%vx%v", p.ImageHeight, p.ImageWidth, p.Turns)
	mutex.Lock()
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
	mutex.Unlock()

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	quit <- true

	close(quit)
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// Gol next state
func nextState(p Params, world [][]byte, start, end int) [][]byte {
	// allocate space
	nextWorld := make([][]byte, end-start)
	for i := range nextWorld {
		nextWorld[i] = make([]byte, p.ImageWidth)
	}

	directions := [8][2]int{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	for row := start; row < end; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			// the alive must be set to 0 everytime when it comes to a different position
			alive := 0
			for _, dir := range directions {
				// + imageHeight make sure the image is connected
				newRow, newCol := (row+dir[0]+p.ImageHeight)%p.ImageHeight, (col+dir[1]+p.ImageWidth)%p.ImageWidth
				if world[newRow][newCol] == 255 {
					alive++
				}
			}
			if world[row][col] == 255 {
				if alive < 2 || alive > 3 {
					nextWorld[row-start][col] = 0
				} else {
					nextWorld[row-start][col] = 255
				}
			} else if world[row][col] == 0 {
				if alive == 3 {
					nextWorld[row-start][col] = 255
				} else {
					nextWorld[row-start][col] = 0
				}
			}
		}
	}

	return nextWorld
}

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	var aliveCell []util.Cell
	for row := 0; row < p.ImageHeight; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			if world[row][col] == 255 {
				aliveCell = append(aliveCell, util.Cell{X: col, Y: row})
			}
		}
	}
	return aliveCell
}
