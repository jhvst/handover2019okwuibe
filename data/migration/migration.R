d = read.csv("data.csv", header=F)
boxplot(d$V3,
    xlab = "sample size: 10",
    ylab = "seconds"
)