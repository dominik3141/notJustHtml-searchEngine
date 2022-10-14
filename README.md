# notJustHtml-searchEngine

## To do
* The rating of a link should also depend on the previous link
* Save perceptual hashes for certain types of files, i.e. for images

## Low priority
* Save keywords in separate table
* Use another database than Sqlite
    * Then you could also you something like A-trees as indexes in order to quickly find the id of a certain link

## Long term
* Reorganize code into modules (so that the graph program can use the same type definitions)
* Implement something like pageRank
* Integrate Virustotal