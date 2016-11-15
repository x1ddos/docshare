# Docshare

This tool facilitates bulk doc sharing. Its initial use case is to
easily share all codelab source docs with the service account for the
publisher app, which is required for the "push to publish" feature,
however, it can be used generally to automate sharing any number of
docs with a given user.

It takes an email address (the party with whom you would like to share
or unshare the requested doc(s)) and one or more doc IDs. For example,
You could share docs doc-id-1 and doc-id-2 with userA, and unshare
the same docs with userB, using the following command:

	docshare -a userA -r userB doc-id-1 doc-id-2

If you have a large set of docs to share you can pipe a stream of
doc IDs into the xargs command, like this:

	`cat file_of_doc_ids | xargs docshare -a email-addr` # share
	`cat file_of_doc_ids | xargs docshare -r email-addr` # unshare

Disclaimer: This is not an official Google product.
