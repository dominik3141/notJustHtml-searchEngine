# notJustHtml-searchEngine
A prototype of a domain specific search engine. I build it with the intention of crawling the web for malware, but it can do all sorts of interesting things, like looking for images that have geotags on them or images that are similar to images in a certain search set.

## To do
* Add a web interface to control and monitor the crawling
* The rating of a link should also depend on the previous link
* Write a better shutdown mechanism, so that all pending operations can still be finished
* Switch to another face recognition backend, the one we are using right now seems to leak memory

## Low priority
* Store files compressed

## Long term
* Reorganize code into modules (so that the graph program can use the same type definitions)
* Implement something like pageRank
* Integrate Virustotal
