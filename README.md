## **Contents**
- [Project Introduction](https://github.com/mattpidden/csa-coursework/tree/dev#project-introduction)
- [Parallel Implementation](https://github.com/mattpidden/csa-coursework/tree/dev#parallel-implementation)
- [Distributed Implementation](https://github.com/mattpidden/csa-coursework/tree/dev#distributed-implementation)
- [Report](https://github.com/mattpidden/csa-coursework/tree/dev#report)
- [Branches](https://github.com/mattpidden/csa-coursework/tree/dev#branches)
- [Group Members](https://github.com/mattpidden/csa-coursework/tree/dev#group-members)

## **Project Introduction**
- This is our Computer Systems A summative coursework. The coursework is worth 80% of our unit mark, 3.33% of our total degree
- It runs over 4 weeks (5 weeks including the reading week) and the deadline for submitting all your work is 30 November 13:00.
- The project is based on the Game of Life. game resides on a 2-valued 2D matrix, i.e. a binary image, where the cells can either be ‘alive’ or ‘dead’. The game evolution is determined by its initial state and requires no further input. Every cell interacts with its eight neighbour pixels: cells that are horizontally, vertically, or diagonally adjacent.

## **Parallel Implementation - DONE**
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
- **Step 4 - DONE**
  - Implement logic to output the state of the board after all turns have completed as a PGM image.
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/raw/master/content/cw_diagrams-Parallel_4.png)
  - To test Step 4 use `go test -v -run=TestPgm`
  - Also use `go test -v` to ensure all tests are passing.
- **Step 5 - DONE**
  - Implement logic to visualise the state of the game using SDL.
  - You will need to use CellFlipped and TurnComplete events to achieve this. (Already implemented in Step 1)
  - Also, implement the following control rules. Note that the goroutine running SDL provides you with a channel containing the relevant keypresses.
    - If `s` is pressed, generate a PGM file with the current state of the board.
    - If `q` is pressed, generate a PGM file with the current state of the board and then terminate the program. Your program should not continue to execute all turns set in gol.Params.Turns.
    - If `p` is pressed, pause the processing and print the current turn that is being processed. If `p` is pressed again resume the processing and print "Continuing". It is not necessary for `q` and `s` to work while the execution is paused.
    - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/raw/master/content/cw_diagrams-Parallel_5.png)
    - Test the visualisation and control rules by running `go run .`
## **Distributed Implementation** [View Branch](https://github.com/mattpidden/csa-coursework/tree/imp/distributed)
- In this stage, we need to create an implementation that uses AWS nodes to calculate the new states of the GOL board, with communication between machines over a network.
- Note view the offical readme for help updating a test file.
- [Offical GitHub page found here](https://github.com/UoB-CSA/gol-skeleton)
- **Step 1**
  - Start with a working single-threaded, single machine implementation (Step 1 of parallel imp).
  - Seperate this implementation into 2 components:
    - A local controller that will handle IO and capturing keypresses from the sdl window, run on a local machine.
    - A server based Gol Engine, responsible for acctually processing the turns of the Gol. This must be able to run on an AWS node. (get it working on local machine first)
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Distributed_1.png)
  - The starting point for this is the local controller telling the Gol Engine to evolve the board, using a single blocking RPC call.
  - Test your implementation using `go test -v -run=TestGol/-1$` on the local controller.
- **Step 2**
  - Get the Gol Engine to report the number of cells alive every 2 seconds to the local controller
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Distributed_2.png)
  - To do this, run a ticker on the local controller, and make an RPC call to the AWS node / worker / broker every 2 seconds. Once received the alive cell count, the local controller should sent that down the `events` channel as an `AliveCellsCount` event.
  - Test your implementation using `go test -v -run=TestAlive` on the local controller.
- **Step 3**
  - Get the local controller to output the state of the board after all turns have been completed as a PGM image.
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Distributed_3.png)
  - Test your implementation using `go test -v -run=TestPgm/-1$` on the local controller.
- **Step 4**
  - Add more functionality to the local controller, allowing it to control the Gol Engine as follows:
    - If `s` is pressed, the controller should generate a PGM file with the current state of the board.
    - If `q` is pressed, close the controller client program without causing an error on the GoL server. A new controller should be able to take over interaction with the GoL engine. Note that you are free to define the nature of how a new controller can take over interaction. Most likely the state will be reset. If you do manage to continue with the previous world this would be considered an extension and a form of fault tolerance.
    - If `k` is pressed, all components of the distributed system are shut down cleanly, and the system outputs a PGM image of the latest state.
    - If `p` is pressed, pause the processing *on the AWS node* and have the *controller* print the current turn that is being processed. If `p` is pressed again resume the processing and have the controller print `"Continuing"`. It is *not* necessary for `q` and `s` to work while the execution is paused.
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Distributed_4.png)
  - Test these controls using `go run .`.
- **Step 5**
  - Split up the Gol board computation across multiple AWS node workers
  - We will need to distribute work between multiple AWS nodes and collect the results in one place. Design the solution so it takes advantge of the possible scalability of many worker machines.
  - I am unsure what this step really means. Should the local controller split up the work, send it to a set number of AWS nodes (is this the equivalent of the threads flag) (this will have to be known ahead of execution in order to start all the relevant nodes manually), and then receive each strip back, stitch the image together and then repeat?
  - Or do we set up a 'broker' on an AWS node that does the splitting up of the image and distributing
  - The offical readme says to 'Make sure to keep the communication between nodes as efficient as possible. For example, consider a halo exchange scheme where only the edges are communicated between the nodes.'
  - View model diagram [here](https://github.com/UoB-CSA/gol-skeleton/blob/master/content/cw_diagrams-Distributed_5.png)
- **Step 6**
  - Reduce the coupling between the local controller and Gol, by implementing a broker
  - The local controller connects to the broker via RPC, and can start the simulation via the main 'Broker' method, that returns the final game state once it is finished.
  - The broker connects to the Gol Workers (other AWS nodes), giving them slices of the game world, which they return the result of after iterating a turn.
  - Note that it is fine to run the broker and local controller both on local machine
- **Extensions**
  - Definitley want to do a few extensions, let's talk over them together and pick them out. Can be found on offical ReadMe linked above.

## **Report**
  - **Overview**
    - Strict maximum of 6 pages
    - Functionality and Design: Outline what functionality you have implemented, which problems you have solved with your implementations and how your program is designed to solve the problems efficiently and effectively.

    - Critical Analysis: Describe the experiments and analysis you carried out. Provide a selection of appropriate results. Keep a history of your implementations and provide benchmark results from various stages. Explain and analyse the benchmark results obtained. Analyse the important factors responsible for the virtues and limitations of your implementations.

    - Make sure your team member’s names and user names appear on page 1 of the report.
  - **Parallel Implementation**
    - Discuss the goroutines you used and how they work together.
    - Explain and analyse the benchmark results obtained. You may want to consider using graphs to visualise your benchmarks.
    - Analyse how your implementation scales as more workers are added.
    - Briefly discuss your methodology for acquiring any results or measurements.
  - **Distributed Implementation**
    - Discuss the system design and reasons for any decisions made. Consider using a diagram to aid your discussion.
    - Explain what data is sent over the network, when, and why it is necessary.  
    - Discuss how your system might scale with the addition of other distributed components.
    - Briefly discuss your methodology for acquiring any results or measurements.
    - Identify how components of your system disappearing (e.g., broken network connections) might affect the overall system and its results.
  - **Benchmarking**
    - To view instructions and all benchmark data view [here](https://docs.google.com/spreadsheets/d/13AwJ_6NnA4v8GKOfqPoSND0-TkccEGJa_2s0QTGeNNQ/edit?usp=sharing)

## **Branches**
  - main branch
    - Will merge `dev` onto `main` once fully complete with everything and ready to submit
  - dev branch
    - Will merge `imp/parallel` onto `dev` once fully finished parallel implementation
    - Will merge `imp/distributed` onto `dev` once fully finished distributed implementation
  - imp/parallel
    - Will merge `feature/step(n)` onto 'imp/parallel' once fully finished with step(n) implementation
  - imp/distributed
    - Will merge `feature/step(n)` onto 'imp/distributed' once fully finished with step(n) implementation
  - extension/extesnion_name
    - Once the distributed implementation is done, any extensions should get work on in a new branch
    - Then we can decide which extensions to merge into the project for submission and which to keep to showcase for the viva only


## **Group Members**
- Matthew Pidden (bb22475@bristol.ac.uk)
- Matthew Cudby (oh22896@bristol.ac.uk)

