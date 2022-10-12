# notJustHtml-searchEngine

## To do
* Move as many variables to the stack as possible to solve the memory problem
* Reuse goroutines instead of spawning new ones
* Use references for the link relations instead of using the whole link
* Make some smart decisions about which links to follow first (i.e. follow links to different sites first)

## Long term
* Save the whole file for certain content-types, i.e. exe files, csv files
* Implement something like pageRank
* Extract keywords (for search) from every site
* The nodeJson column is a bit of a problem, find a better solution for this
* Use another database than Sqlite
    * Then you could also you something like A-trees as indexes in order to quickly find the id of a certain link
* Integrate Virustotal
* save perceptual hashes for certain types of files, i.e. for images