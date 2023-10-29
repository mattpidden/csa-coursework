## **Contents**
- [Project Introduction](https://github.com/mattpidden/csa-coursework/tree/dev#project-introduction)
- [Parallel Implementation](https://github.com/mattpidden/csa-coursework/tree/dev#parallel-implementation)
- [Distributed Implementation](https://github.com/mattpidden/csa-coursework/tree/dev#distributed-implementation)
- [Report](https://github.com/mattpidden/csa-coursework/tree/dev#report)
- [Branches](https://github.com/mattpidden/csa-coursework/tree/dev#branches)
- [Group Members](https://github.com/mattpidden/csa-coursework/tree/dev#group-members)

## **Project Introduction**
- This is our Computer Systems A summative coursework. The coursework is worth 80% of our unit mark. 
- It runs over 4 weeks (5 weeks including the reading week) and the deadline for submitting all your work is 30 November 13:00.
- The project is based on the Game of Life. game resides on a 2-valued 2D matrix, i.e. a binary image, where the cells can either be ‘alive’ or ‘dead’. The game evolution is determined by its initial state and requires no further input. Every cell interacts with its eight neighbour pixels: cells that are horizontally, vertically, or diagonally adjacent.

## **Parallel Implementation** [View Branch](https://github.com/mattpidden/csa-coursework/tree/imp/parallel)
- In this stage, we are required to write code to evolve Game of Life using multiple worker goroutines on a single machine.
- Your Game of Life code will interact with the user or the unit tests using the events channel.
- [Offical GitHub page found here](https://github.com/UoB-CSA/gol-skeleton)
- **Step 1 - DONE**
  - Starting with a single-threaded implementation that will serve as a starting point for subsequent steps
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Parallel_1.png)
  - To test Step 1 use `go test -v -run=TestGol/-1$`
- **Step 2 - DONE** 
  - Parallelise your Game of Life so that it uses worker threads to calculate the new state of the board. You should implement a distributor that tasks different worker threads to operate on different parts of the image in parallel.
  - We are going to follow the same method as the median filter lab task.
  - The number of worker threads you should create is specified in `gol.Params.Threads`
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Parallel_2.png)
  - To test Step 2 use `go test -v -run=TestGol`
- **Step 3 - DONE**
  - Using a ticker, implement the reporting of the number of cells that are still alive every 2 seconds.
  - To report the count use the AliveCellsCount event.
  - Also send the TurnComplete event after each complete iteration. (Already implemented in Step 1)
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/raw/master/content/cw_diagrams-Parallel_3.png)
  - To test Step 3 use `go test -v -run=TestAlive`
- **Step 4 - IN PROGRESS**
  - Implement logic to output the state of the board after all turns have completed as a PGM image.
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/raw/master/content/cw_diagrams-Parallel_4.png)
  - To test Step 4 use `go test -v -run=TestPgm`
  - Also use `go test -v` to ensure all tests are passing.
- **Step 5**
  - Implement logic to visualise the state of the game using SDL.
  - You will need to use CellFlipped and TurnComplete events to achieve this. (Already implemented in Step 1)
  - Also, implement the following control rules. Note that the goroutine running SDL provides you with a channel containing the relevant keypresses.
    - If `s` is pressed, generate a PGM file with the current state of the board.
    - If `q` is pressed, generate a PGM file with the current state of the board and then terminate the program. Your program should not continue to execute all turns set in gol.Params.Turns.
    - If `p` is pressed, pause the processing and print the current turn that is being processed. If `p` is pressed again resume the processing and print "Continuing". It is not necessary for `q` and `s` to work while the execution is paused.
    - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/raw/master/content/cw_diagrams-Parallel_5.png)
    - Test the visualisation and control rules by running `go run .`
## **Distributed Implementation** [View Branch](https://github.com/mattpidden/csa-coursework/tree/imp/distributed)
  - Waiting for release of this part.

## **Report**
  - Awaiting details for this task.

## **Branches**
  - main branch
    - Will merge `dev` onto `main` once fully complete with everything
  - dev branch
    - Will merge `imp/parallel` onto `dev` once fully finished parallel implementation
    - Will merge `imp/distributed` onto `dev` once fully finished distributed implementation
  - imp/parallel
    - Will merge `feature/step(n)` onto 'imp/parallel' once fully finished with step(n) implementation
  - imp/distributed
    - Will merge `feature/step(n)` onto 'imp/distributed' once fully finished with step(n) implementation


## **Group Members**
- Matthew Pidden (bb22475@bristol.ac.uk)
- Matthew Cudby (oh22896@bristol.ac.uk)

