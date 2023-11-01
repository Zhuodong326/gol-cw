package gol

import (
	"fmt"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// The ioInput is just a const for operation
	// It determines the operation to do
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	// Initialize the state
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput

		}
	}
	turn := 0
	// TODO: Execute all turns of the Game of Life.
	for ; turn < p.Turns; turn++ {
		world = nextState(p, world)
		c.events <- TurnComplete{turn}
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, calculateAliveCells(p, world)}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// Gol next state
func nextState(p Params, world [][]byte) [][]byte {
	// allocate space
	nextWorld := make([][]byte, p.ImageHeight)
	for i := range nextWorld {
		nextWorld[i] = make([]byte, p.ImageWidth)
	}

	directions := [8][2]int{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	for row := 0; row < p.ImageHeight; row++ {
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
					nextWorld[row][col] = 0
				} else {
					nextWorld[row][col] = 255
				}
			} else if world[row][col] == 0 {
				if alive == 3 {
					nextWorld[row][col] = 255
				} else {
					nextWorld[row][col] = 0
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
