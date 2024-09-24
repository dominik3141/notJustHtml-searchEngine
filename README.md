# notJustHtml-searchEngine

A sophisticated domain-specific search engine prototype designed for versatile web crawling applications. While initially conceived for malware detection, this engine demonstrates remarkable flexibility, capable of tasks such as identifying geotagged images or finding visually similar images based on a given search set.

## Key Features

- Adaptive crawling mechanism for efficient web traversal
- Face recognition capabilities for image analysis
- Perceptual image hashing for similarity detection
- EXIF data extraction from images
- Bloom filter for efficient URL management
- Redis-based queue system for prioritized crawling
- PostgreSQL database for robust data storage
- Parallel crawling with configurable number of workers
- Debug mode for detailed logging and analysis
- Optional Chrome integration for JavaScript-heavy websites

## To-Do List

1. Enhance crawling mechanism for better stability and efficiency
2. Develop a web interface for crawling control and monitoring
3. Implement link rating based on the previous link
4. Improve shutdown mechanism to complete pending operations
5. Replace current face recognition backend to address memory leaks

### Low Priority Tasks

- Implement file compression for storage optimization
- Integrate an ML library for object and emotion detection

### Long-term Goals

- Reorganize code into modules for improved maintainability
- Implement a PageRank-like algorithm
- Integrate VirusTotal API for enhanced malware detection

