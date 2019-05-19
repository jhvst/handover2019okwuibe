d = read.csv("e2e.csv", header=F)
boxplot(d$V3,
    xlab = "sample size: 10",
    ylab = "seconds"
)